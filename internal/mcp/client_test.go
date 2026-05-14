package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildFakeServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "fake_server.go")
	code := `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type req struct {
	JSONRPC string          ` + "`json:\"jsonrpc\"`" + `
	ID      int64           ` + "`json:\"id\"`" + `
	Method  string          ` + "`json:\"method\"`" + `
	Params  json.RawMessage ` + "`json:\"params\"`" + `
}

func main() {
	scan := bufio.NewScanner(os.Stdin)
	scan.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scan.Scan() {
		var r req
		if err := json.Unmarshal(scan.Bytes(), &r); err != nil {
			continue
		}
		if r.ID == 0 {
			continue
		}
		switch r.Method {
		case "initialize":
			resp := map[string]interface{}{
				"jsonrpc": "2.0", "id": r.ID,
				"result": map[string]interface{}{
					"protocolVersion": "2025-06-18",
					"capabilities":    map[string]interface{}{},
					"serverInfo":      map[string]interface{}{"name": "fake", "version": "0.0"},
				},
			}
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
		case "tools/list":
			resp := map[string]interface{}{
				"jsonrpc": "2.0", "id": r.ID,
				"result": map[string]interface{}{
					"tools": []map[string]interface{}{
						{
							"name":        "echo",
							"description": "echo back",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"text": map[string]interface{}{"type": "string", "description": "what to echo"},
								},
								"required": []interface{}{"text"},
							},
						},
					},
				},
			}
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
		case "tools/call":
			var p struct {
				Name      string                 ` + "`json:\"name\"`" + `
				Arguments map[string]interface{} ` + "`json:\"arguments\"`" + `
			}
			_ = json.Unmarshal(r.Params, &p)
			txt, _ := p.Arguments["text"].(string)
			resp := map[string]interface{}{
				"jsonrpc": "2.0", "id": r.ID,
				"result": map[string]interface{}{
					"content": []map[string]interface{}{
						{"type": "text", "text": "echo:" + txt},
					},
					"isError": false,
				},
			}
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
		default:
			resp := map[string]interface{}{
				"jsonrpc": "2.0", "id": r.ID,
				"error": map[string]interface{}{"code": -32601, "message": "method not found"},
			}
			b, _ := json.Marshal(resp)
			fmt.Println(string(b))
		}
	}
}
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatalf("write fake server: %v", err)
	}
	bin := filepath.Join(dir, "fake_server")
	cmd := exec.Command("go", "build", "-o", bin, src)
	if err := cmd.Run(); err != nil {
		t.Fatalf("build fake server: %v", err)
	}
	return bin
}

func TestClient_InitializeListCall(t *testing.T) {
	bin := buildFakeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := NewClient(ctx, "fake", ServerConfig{Command: bin})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer c.Close()

	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	infos, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(infos) != 1 || infos[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", infos)
	}
	if infos[0].Description != "echo back" {
		t.Fatalf("missing description")
	}

	out, err := c.CallTool(ctx, "echo", map[string]interface{}{"text": "hi"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if out != "echo:hi" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestClient_CallUnknownMethod(t *testing.T) {
	bin := buildFakeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := NewClient(ctx, "fake", ServerConfig{Command: bin})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer c.Close()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	_, err = c.call(ctx, "bogus/method", map[string]interface{}{})
	if err == nil {
		t.Fatalf("expected error for unknown method")
	}
	if !strings.Contains(err.Error(), "method not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfig_MissingFileReturnsEmpty(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file must not error, got %v", err)
	}
	if len(cfg.MCPServers) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestLoadConfig_ParsesClaudeDesktopShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcps.json")
	raw := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			"filesystem": map[string]interface{}{
				"command": "npx",
				"args":    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
				"env":     map[string]string{"FOO": "bar"},
			},
		},
	}
	b, _ := json.Marshal(raw)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fs, ok := cfg.MCPServers["filesystem"]
	if !ok {
		t.Fatalf("filesystem server missing")
	}
	if fs.Command != "npx" {
		t.Fatalf("wrong command: %q", fs.Command)
	}
	if len(fs.Args) != 3 || fs.Args[2] != "/tmp" {
		t.Fatalf("wrong args: %+v", fs.Args)
	}
	if fs.Env["FOO"] != "bar" {
		t.Fatalf("wrong env: %+v", fs.Env)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"simple":        "simple",
		"with space":    "with_space",
		"weird@chars!":  "weird_chars_",
		"keep-dash_1":   "keep-dash_1",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
