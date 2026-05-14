package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeEchoTool struct{}

func (f *fakeEchoTool) Schema() *Schema {
	return &Schema{
		Name:        "echo_tool",
		Description: "Echoes back the text argument",
		Fields: []FieldSpec{
			{Name: "text", Kind: KindString, Required: true, Description: "Text to echo"},
		},
	}
}

func (f *fakeEchoTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	text := args["text"].(string)
	return text, nil
}

type fakeSlowTool struct{}

func (f *fakeSlowTool) Schema() *Schema {
	return &Schema{
		Name:        "slow_tool",
		Description: "A tool that sleeps",
		Fields: []FieldSpec{
			{Name: "duration", Kind: KindNumber, Required: true, Description: "Sleep duration in ms"},
		},
	}
}

func (f *fakeSlowTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	duration := time.Duration(args["duration"].(float64)) * time.Millisecond
	select {
	case <-time.After(duration):
		return "done", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type fakeLargeTool struct{}

func (f *fakeLargeTool) Schema() *Schema {
	return &Schema{
		Name:        "large_tool",
		Description: "A tool that returns large output",
		Fields:      []FieldSpec{},
	}
}

func (f *fakeLargeTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	output := ""
	for i := 0; i < 20000; i++ {
		output += "x"
	}
	return output, nil
}

type fakeAdminTool struct{}

func (f *fakeAdminTool) Schema() *Schema {
	return &Schema{
		Name:        "admin_tool",
		Description: "An admin-only tool",
		Fields: []FieldSpec{
			{Name: "action", Kind: KindString, Required: true, Description: "Action to perform"},
		},
	}
}

func (f *fakeAdminTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	return "admin action executed", nil
}

func TestRegisterValidatesSchema(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	badTool := &fakeEchoTool{}
	def := &ToolDefinition{
		Tool:    badTool,
		Timeout: 5 * time.Second,
	}

	registry.defs["echo_tool"] = def

	badDef := &ToolDefinition{
		Tool: badTool,
	}

	err := registry.Register(badDef)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
	if !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool, got %v", err)
	}
}

func TestRegisterDuplicateRejected(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	def := &ToolDefinition{
		Tool:    &fakeEchoTool{},
		Timeout: 5 * time.Second,
	}

	if err := registry.Register(def); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	err := registry.Register(def)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
	if !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool, got %v", err)
	}
}

func TestExecuteUnknownTool(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	req := ExecuteRequest{
		GuildID: 123,
		UserID:  456,
		Tool:    "nonexistent",
		Args:    map[string]interface{}{},
		Caller:  CallerContext{IsAdmin: false},
	}

	result := registry.Execute(context.Background(), req)
	if result.Err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !errors.Is(result.Err, ErrUnknownTool) {
		t.Fatalf("expected ErrUnknownTool, got %v", result.Err)
	}

	events := audit.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Status != "unknown_tool" {
		t.Fatalf("expected status unknown_tool, got %s", events[0].Status)
	}
}

func TestExecuteAdminOnlyDeniesNonAdmin(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	def := &ToolDefinition{
		Tool:      &fakeAdminTool{},
		Timeout:   5 * time.Second,
		AdminOnly: true,
	}
	if err := registry.Register(def); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	req := ExecuteRequest{
		GuildID: 123,
		UserID:  456,
		Tool:    "admin_tool",
		Args:    map[string]interface{}{"action": "delete"},
		Caller:  CallerContext{IsAdmin: false},
	}

	result := registry.Execute(context.Background(), req)
	if result.Err == nil {
		t.Fatal("expected error for non-admin access")
	}
	if !errors.Is(result.Err, ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", result.Err)
	}

	events := audit.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Status != "permission_denied" {
		t.Fatalf("expected status permission_denied, got %s", events[0].Status)
	}
}

func TestExecuteAdminOnlyAllowsAdmin(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	def := &ToolDefinition{
		Tool:      &fakeAdminTool{},
		Timeout:   5 * time.Second,
		AdminOnly: true,
	}
	if err := registry.Register(def); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	req := ExecuteRequest{
		GuildID: 123,
		UserID:  456,
		Tool:    "admin_tool",
		Args:    map[string]interface{}{"action": "delete"},
		Caller:  CallerContext{IsAdmin: true},
	}

	result := registry.Execute(context.Background(), req)
	if result.Err != nil {
		t.Fatalf("expected no error for admin access, got %v", result.Err)
	}
	if result.Output != "admin action executed" {
		t.Fatalf("expected output 'admin action executed', got %s", result.Output)
	}

	events := audit.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Status != "ok" {
		t.Fatalf("expected status ok, got %s", events[0].Status)
	}
}

func TestExecuteInvalidArgsReturnsError(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	def := &ToolDefinition{
		Tool:    &fakeEchoTool{},
		Timeout: 5 * time.Second,
	}
	if err := registry.Register(def); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	req := ExecuteRequest{
		GuildID: 123,
		UserID:  456,
		Tool:    "echo_tool",
		Args:    map[string]interface{}{"text": 123},
		Caller:  CallerContext{IsAdmin: false},
	}

	result := registry.Execute(context.Background(), req)
	if result.Err == nil {
		t.Fatal("expected error for invalid args")
	}
	if !errors.Is(result.Err, ErrInvalidArgType) {
		t.Fatalf("expected ErrInvalidArgType, got %v", result.Err)
	}

	events := audit.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Status != "invalid_args" {
		t.Fatalf("expected status invalid_args, got %s", events[0].Status)
	}
}

func TestExecuteTimeoutReturnsErrTimeout(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	def := &ToolDefinition{
		Tool:    &fakeSlowTool{},
		Timeout: 100 * time.Millisecond,
	}
	if err := registry.Register(def); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	req := ExecuteRequest{
		GuildID: 123,
		UserID:  456,
		Tool:    "slow_tool",
		Args:    map[string]interface{}{"duration": float64(500)},
		Caller:  CallerContext{IsAdmin: false},
	}

	result := registry.Execute(context.Background(), req)
	if result.Err == nil {
		t.Fatal("expected error for timeout")
	}
	if !errors.Is(result.Err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", result.Err)
	}

	events := audit.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Status != "timeout" {
		t.Fatalf("expected status timeout, got %s", events[0].Status)
	}
}

func TestExecuteTruncatesLargeOutput(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	def := &ToolDefinition{
		Tool:      &fakeLargeTool{},
		Timeout:   5 * time.Second,
		MaxOutput: 100,
	}
	if err := registry.Register(def); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	req := ExecuteRequest{
		GuildID: 123,
		UserID:  456,
		Tool:    "large_tool",
		Args:    map[string]interface{}{},
		Caller:  CallerContext{IsAdmin: false},
	}

	result := registry.Execute(context.Background(), req)
	if result.Err != nil {
		t.Fatalf("expected no error, got %v", result.Err)
	}
	if len(result.Output) != 100 {
		t.Fatalf("expected output length 100, got %d", len(result.Output))
	}
	if !result.Truncated {
		t.Fatal("expected Truncated to be true")
	}

	events := audit.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Status != "truncated" {
		t.Fatalf("expected status truncated, got %s", events[0].Status)
	}
}

func TestExecuteHappyPathAudit(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	def := &ToolDefinition{
		Tool:    &fakeEchoTool{},
		Timeout: 5 * time.Second,
	}
	if err := registry.Register(def); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	req := ExecuteRequest{
		GuildID: 123,
		UserID:  456,
		Tool:    "echo_tool",
		Args:    map[string]interface{}{"text": "hello world"},
		Caller:  CallerContext{IsAdmin: false},
	}

	result := registry.Execute(context.Background(), req)
	if result.Err != nil {
		t.Fatalf("expected no error, got %v", result.Err)
	}
	if result.Output != "hello world" {
		t.Fatalf("expected output 'hello world', got %s", result.Output)
	}
	if result.Truncated {
		t.Fatal("expected Truncated to be false")
	}

	events := audit.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}

	evt := events[0]
	if evt.GuildID != 123 {
		t.Fatalf("expected GuildID 123, got %d", evt.GuildID)
	}
	if evt.UserID != 456 {
		t.Fatalf("expected UserID 456, got %d", evt.UserID)
	}
	if evt.Tool != "echo_tool" {
		t.Fatalf("expected Tool echo_tool, got %s", evt.Tool)
	}
	if evt.Status != "ok" {
		t.Fatalf("expected Status ok, got %s", evt.Status)
	}
	if evt.Duration == 0 {
		t.Fatal("expected Duration > 0")
	}
	if evt.Error != "" {
		t.Fatalf("expected no error message, got %s", evt.Error)
	}
}

func TestRegistry_OpenAIFunctions_SkipsAdminTools(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	normalDef := &ToolDefinition{
		Tool:      &fakeEchoTool{},
		Timeout:   5 * time.Second,
		AdminOnly: false,
	}
	if err := registry.Register(normalDef); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	adminDef := &ToolDefinition{
		Tool:      &fakeAdminTool{},
		Timeout:   5 * time.Second,
		AdminOnly: true,
	}
	if err := registry.Register(adminDef); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	result := registry.OpenAIFunctions()

	if len(result) != 1 {
		t.Fatalf("expected 1 function, got %d", len(result))
	}

	funcObj, ok := result[0]["function"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected function to be map[string]interface{}, got %T", result[0]["function"])
	}

	if funcObj["name"] != "echo_tool" {
		t.Fatalf("expected name='echo_tool', got %v", funcObj["name"])
	}
}

func TestRegistry_OpenAIFunctions_RoundTrip(t *testing.T) {
	audit := &InMemoryAudit{}
	registry := NewRegistry(audit)

	def := &ToolDefinition{
		Tool:    &fakeEchoTool{},
		Timeout: 5 * time.Second,
	}
	if err := registry.Register(def); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	result := registry.OpenAIFunctions()

	if len(result) != 1 {
		t.Fatalf("expected 1 function, got %d", len(result))
	}

	jsonBytes, err := json.Marshal(result[0])
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled["type"] != "function" {
		t.Fatalf("expected type='function' after round-trip, got %v", unmarshaled["type"])
	}

	funcObj, ok := unmarshaled["function"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected function to be map[string]interface{} after round-trip, got %T", unmarshaled["function"])
	}

	if funcObj["name"] != "echo_tool" {
		t.Fatalf("expected name='echo_tool' after round-trip, got %v", funcObj["name"])
	}

	params, ok := funcObj["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parameters to be map[string]interface{} after round-trip, got %T", funcObj["parameters"])
	}

	if params["type"] != "object" {
		t.Fatalf("expected parameters.type='object' after round-trip, got %v", params["type"])
	}
}
