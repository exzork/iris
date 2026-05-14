package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/eko/iris-bot/internal/tools"
)

// Manager owns the lifecycle of all configured MCP servers and the adapters
// that expose their tools through iris-bot's tools.Registry.
type Manager struct {
	clients map[string]*Client
}

// NewManager spawns every server listed in cfg, initializes them, and
// registers their tools into reg. Tool names are prefixed with "<server>_"
// to keep the global tool namespace flat and collision-free.
//
// A failure on one server does not block the others: the manager logs the
// failure and continues. Returns the set of successfully registered tools
// and the manager handle for shutdown.
func NewManager(ctx context.Context, cfg Config, reg *tools.Registry) (*Manager, int, error) {
	m := &Manager{clients: make(map[string]*Client)}
	registered := 0

	for name, serverCfg := range cfg.MCPServers {
		client, err := NewClient(ctx, name, serverCfg)
		if err != nil {
			slog.Warn("mcp_server_spawn_failed", "server", name, "err", err)
			continue
		}
		initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := client.Initialize(initCtx); err != nil {
			cancel()
			slog.Warn("mcp_server_init_failed", "server", name, "err", err)
			_ = client.Close()
			continue
		}
		cancel()

		listCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		toolInfos, err := client.ListTools(listCtx)
		cancel()
		if err != nil {
			slog.Warn("mcp_server_list_tools_failed", "server", name, "err", err)
			_ = client.Close()
			continue
		}

		m.clients[name] = client
		for _, info := range toolInfos {
			adapter := &toolAdapter{
				client:     client,
				mcpName:    info.Name,
				schema:     buildSchema(name, info),
				origSchema: info.InputSchema,
			}
			def := &tools.ToolDefinition{
				Tool:      adapter,
				Timeout:   30 * time.Second,
				MaxOutput: 16 * 1024,
			}
			if err := reg.Register(def); err != nil {
				slog.Warn("mcp_tool_register_failed", "server", name, "tool", info.Name, "err", err)
				continue
			}
			registered++
			slog.Info("mcp_tool_registered", "server", name, "tool", info.Name, "registered_as", adapter.schema.Name)
		}
		slog.Info("mcp_server_ready", "server", name, "tools", len(toolInfos))
	}

	return m, registered, nil
}

// Close shuts down every MCP server subprocess. Safe to call multiple times.
func (m *Manager) Close() error {
	for name, c := range m.clients {
		if err := c.Close(); err != nil {
			slog.Warn("mcp_server_close_error", "server", name, "err", err)
		}
	}
	m.clients = nil
	return nil
}

// LoadConfig reads an mcps.json file using the Claude Desktop schema
// {"mcpServers": {"name": {"command": "...", "args": [...], "env": {...}}}}.
// A missing file returns an empty config and no error so MCP is opt-in.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("mcp: read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("mcp: parse %s: %w", path, err)
	}
	return cfg, nil
}

type toolAdapter struct {
	client     *Client
	mcpName    string
	schema     *tools.Schema
	origSchema map[string]interface{}
}

func (a *toolAdapter) Schema() *tools.Schema { return a.schema }

func (a *toolAdapter) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	return a.client.CallTool(ctx, a.mcpName, args)
}

// buildSchema converts an MCP tool's JSON Schema into the iris-bot
// tools.Schema shape. The registered name is prefixed with the server name
// so tools from different servers never collide, and the prefix is
// sanitized to satisfy OpenAI's function-name character set.
func buildSchema(server string, info ToolInfo) *tools.Schema {
	prefixed := sanitize(server) + "_" + sanitize(info.Name)
	s := &tools.Schema{
		Name:        prefixed,
		Description: info.Description,
	}

	props, _ := info.InputSchema["properties"].(map[string]interface{})
	requiredList, _ := info.InputSchema["required"].([]interface{})
	requiredSet := make(map[string]bool, len(requiredList))
	for _, r := range requiredList {
		if k, ok := r.(string); ok {
			requiredSet[k] = true
		}
	}

	for name, raw := range props {
		prop, _ := raw.(map[string]interface{})
		typeStr, _ := prop["type"].(string)
		desc, _ := prop["description"].(string)
		s.Fields = append(s.Fields, tools.FieldSpec{
			Name:        name,
			Kind:        jsonSchemaToKind(typeStr),
			Required:    requiredSet[name],
			Description: desc,
		})
	}
	return s
}

func jsonSchemaToKind(t string) tools.Kind {
	switch strings.ToLower(t) {
	case "string":
		return tools.KindString
	case "number", "integer":
		return tools.KindNumber
	case "boolean":
		return tools.KindBool
	case "object":
		return tools.KindObject
	case "array":
		return tools.KindArray
	default:
		return tools.KindString
	}
}

// sanitize replaces characters OpenAI/Anthropic function-calling forbids in
// tool names (must match ^[a-zA-Z0-9_-]+$) with underscores.
func sanitize(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_', c == '-':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	return string(b)
}
