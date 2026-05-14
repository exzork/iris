// Package mcp implements a Model Context Protocol (MCP) client for stdio
// transport. It spawns MCP servers as subprocesses and exposes their tools
// through iris-bot's existing tools.Registry.
//
// This client speaks MCP's stdio framing: newline-delimited JSON-RPC 2.0
// messages, one per line on stdin/stdout. It performs the initialize
// handshake, discovers tools via tools/list, and dispatches tool calls
// via tools/call.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const protocolVersion = "2025-06-18"

type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type Config struct {
	MCPServers    map[string]ServerConfig `json:"mcpServers"`
	SlashCommands json.RawMessage         `json:"slashCommands,omitempty"`
}

type ToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message)
}

// Client is a single running MCP server subprocess plus its JSON-RPC loop.
type Client struct {
	name string
	cmd  *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	nextID atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan rpcResponse
	closed  bool

	done chan struct{}
}

// NewClient spawns the MCP server described by cfg and starts its read loop.
// The returned client must have Initialize called before any other methods.
func NewClient(ctx context.Context, name string, cfg ServerConfig) (*Client, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("mcp: server %q has no command", name)
	}
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
		env := cmd.Env
		for k, v := range cfg.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe for %q: %w", name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe for %q: %w", name, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stderr pipe for %q: %w", name, err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start %q: %w", name, err)
	}

	c := &Client{
		name:    name,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		pending: make(map[int64]chan rpcResponse),
		done:    make(chan struct{}),
	}

	go c.readLoop()
	go drainStderr(name, stderr)

	return c, nil
}

func drainStderr(name string, r io.Reader) {
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scan.Scan() {
		slog.Debug("mcp_server_stderr", "server", name, "line", scan.Text())
	}
}

func (c *Client) readLoop() {
	defer close(c.done)
	scan := bufio.NewScanner(c.stdout)
	scan.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scan.Scan() {
		line := scan.Bytes()
		if len(line) == 0 {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			slog.Debug("mcp_unmarshal_error", "server", c.name, "err", err, "line", string(line))
			continue
		}
		if resp.ID == 0 {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- resp
			close(ch)
		}
	}
}

func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan rpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("mcp: client closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal %s: %w", method, err)
	}
	payload = append(payload, '\n')
	if _, err := c.stdin.Write(payload); err != nil {
		return nil, fmt.Errorf("mcp: write %s: %w", method, err)
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

func (c *Client) notify(method string, params interface{}) error {
	n := rpcNotification{JSONRPC: "2.0", Method: method, Params: params}
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("mcp: marshal notify %s: %w", method, err)
	}
	payload = append(payload, '\n')
	_, err = c.stdin.Write(payload)
	return err
}

// Initialize performs the MCP handshake: initialize request + notifications/initialized.
func (c *Client) Initialize(ctx context.Context) error {
	params := map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "iris-bot",
			"version": "1.0.0",
		},
	}
	_, err := c.call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("mcp: initialize %q: %w", c.name, err)
	}
	if err := c.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("mcp: notify initialized %q: %w", c.name, err)
	}
	return nil
}

// ListTools returns the MCP server's advertised tools via tools/list.
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	var tools []ToolInfo
	cursor := ""
	for {
		var params map[string]interface{}
		if cursor != "" {
			params = map[string]interface{}{"cursor": cursor}
		}
		raw, err := c.call(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Tools      []ToolInfo `json:"tools"`
			NextCursor string     `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("mcp: tools/list decode %q: %w", c.name, err)
		}
		tools = append(tools, resp.Tools...)
		if resp.NextCursor == "" {
			return tools, nil
		}
		cursor = resp.NextCursor
	}
}

// CallTool invokes a tool on this server and returns the text content joined.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	raw, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("mcp: tools/call decode %q: %w", c.name, err)
	}
	var out string
	for i, part := range resp.Content {
		if part.Type != "text" {
			continue
		}
		if i > 0 {
			out += "\n"
		}
		out += part.Text
	}
	if resp.IsError {
		return out, fmt.Errorf("mcp: tool %q reported error: %s", name, out)
	}
	return out, nil
}

// Name returns the configured server name.
func (c *Client) Name() string { return c.name }

// Close shuts the MCP server down and releases the subprocess.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	_ = c.stdin.Close()

	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
	}

	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	return nil
}
