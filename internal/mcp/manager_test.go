package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/tools"
)

func TestManager_RegistersToolsIntoRegistry(t *testing.T) {
	bin := buildFakeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reg := tools.NewRegistry(nil)
	cfg := Config{MCPServers: map[string]ServerConfig{
		"fake": {Command: bin},
	}}
	m, count, err := NewManager(ctx, cfg, reg)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer m.Close()

	if count != 1 {
		t.Fatalf("expected 1 registered tool, got %d", count)
	}
	names := reg.List()
	if len(names) != 1 || names[0] != "fake_echo" {
		t.Fatalf("expected [fake_echo], got %v", names)
	}

	def, ok := reg.Get("fake_echo")
	if !ok {
		t.Fatalf("fake_echo not retrievable")
	}
	if def.Tool.Schema().Description != "echo back" {
		t.Fatalf("wrong description: %q", def.Tool.Schema().Description)
	}

	result := reg.Execute(ctx, tools.ExecuteRequest{
		Tool: "fake_echo",
		Args: map[string]interface{}{"text": "hello"},
	})
	if result.Err != nil {
		t.Fatalf("execute error: %v", result.Err)
	}
	if result.Output != "echo:hello" {
		t.Fatalf("wrong output: %q", result.Output)
	}
}

func TestManager_MissingServerSkippedGracefully(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	reg := tools.NewRegistry(nil)
	cfg := Config{MCPServers: map[string]ServerConfig{
		"missing": {Command: "/definitely/not/a/real/path/iris-mcp-server"},
	}}
	m, count, err := NewManager(ctx, cfg, reg)
	if err != nil {
		t.Fatalf("manager must not hard-fail when one server is missing, got %v", err)
	}
	defer m.Close()
	if count != 0 {
		t.Fatalf("no tools should register for unstartable server, got %d", count)
	}
}

func TestBuildSchema_PrefixesNameAndTranslatesRequiredFields(t *testing.T) {
	info := ToolInfo{
		Name:        "weather",
		Description: "get weather",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"city":   map[string]interface{}{"type": "string", "description": "City"},
				"units":  map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"city"},
		},
	}
	s := buildSchema("open weather", info)
	if s.Name != "open_weather_weather" {
		t.Fatalf("name prefix/sanitize wrong: %q", s.Name)
	}
	var cityField, unitsField *tools.FieldSpec
	for i := range s.Fields {
		f := &s.Fields[i]
		switch f.Name {
		case "city":
			cityField = f
		case "units":
			unitsField = f
		}
	}
	if cityField == nil || !cityField.Required {
		t.Fatalf("city must be required")
	}
	if cityField.Kind != tools.KindString {
		t.Fatalf("city kind %q != string", cityField.Kind)
	}
	if unitsField == nil || unitsField.Required {
		t.Fatalf("units must not be required")
	}
}
