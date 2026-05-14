package modelswitch

import (
	"context"
	"strings"
	"testing"

	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/mcp"
)

type fakeStore struct {
	data map[string]string
}

func (f *fakeStore) Get(ctx context.Context, key string) (string, bool, error) {
	v, ok := f.data[key]
	return v, ok, nil
}

func (f *fakeStore) Set(ctx context.Context, key, value string, updatedBy int64) error {
	f.data[key] = value
	return nil
}

func (f *fakeStore) Delete(ctx context.Context, key string) error {
	delete(f.data, key)
	return nil
}

type fakeAudit struct {
	events []auditEvent
}

type auditEvent struct {
	eventType string
	entityID  string
	actor     int64
	changes   map[string]interface{}
}

func (f *fakeAudit) Log(ctx context.Context, guildID, userID int64, eventType, entityType, entityID string, changes map[string]interface{}) error {
	f.events = append(f.events, auditEvent{eventType: eventType, entityID: entityID, actor: userID, changes: changes})
	return nil
}

func newResolver() (*llm.ModelResolver, *fakeStore) {
	store := &fakeStore{data: map[string]string{}}
	validate := func(m string) error {
		if strings.HasPrefix(m, "bad/") {
			return errTestInvalid
		}
		return nil
	}
	r := llm.NewModelResolver(store, validate, "fallback-default", "fallback-strong", "fallback-router")
	_ = r.Load(context.Background())
	return r, store
}

var errTestInvalid = &validationError{msg: "invalid"}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func TestSetToolRejectsNonOwner(t *testing.T) {
	resolver, _ := newResolver()
	tool := NewSetTool(resolver, 42, nil)

	ctx := mcp.WithCallerUserID(context.Background(), 999)
	_, err := tool.Run(ctx, map[string]interface{}{"tier": "default", "model": "kr/new"})
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected owner gate error, got: %v", err)
	}
}

func TestSetToolRejectsMissingOwner(t *testing.T) {
	resolver, _ := newResolver()
	tool := NewSetTool(resolver, 0, nil)

	ctx := mcp.WithCallerUserID(context.Background(), 42)
	_, err := tool.Run(ctx, map[string]interface{}{"tier": "default", "model": "kr/new"})
	if err == nil || !strings.Contains(err.Error(), "owner is not configured") {
		t.Fatalf("expected unconfigured-owner error, got: %v", err)
	}
}

func TestSetToolHappyPathPersistsAndAudits(t *testing.T) {
	resolver, store := newResolver()
	audit := &fakeAudit{}
	tool := NewSetTool(resolver, 42, audit)

	ctx := mcp.WithCallerUserID(context.Background(), 42)
	out, err := tool.Run(ctx, map[string]interface{}{"tier": "default", "model": "kr/new-default"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "kr/new-default") {
		t.Errorf("response missing new model: %q", out)
	}
	if got := resolver.Default(); got != "kr/new-default" {
		t.Errorf("resolver Default = %q", got)
	}
	if got := store.data[llm.SettingKeyModelDefault]; got != "kr/new-default" {
		t.Errorf("store not updated: %v", store.data)
	}
	if len(audit.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(audit.events))
	}
	if audit.events[0].eventType != "model_switch" {
		t.Errorf("audit event type = %q", audit.events[0].eventType)
	}
	if audit.events[0].actor != 42 {
		t.Errorf("audit actor = %d, want 42", audit.events[0].actor)
	}
	if audit.events[0].changes["previous"] != "fallback-default" {
		t.Errorf("audit previous = %v", audit.events[0].changes["previous"])
	}
}

func TestSetToolRejectsInvalidTier(t *testing.T) {
	resolver, _ := newResolver()
	tool := NewSetTool(resolver, 42, nil)

	ctx := mcp.WithCallerUserID(context.Background(), 42)
	_, err := tool.Run(ctx, map[string]interface{}{"tier": "weird", "model": "kr/new"})
	if err == nil || !strings.Contains(err.Error(), "unknown tier") {
		t.Fatalf("expected unknown-tier error, got: %v", err)
	}
}

func TestSetToolRejectsMissingArgs(t *testing.T) {
	resolver, _ := newResolver()
	tool := NewSetTool(resolver, 42, nil)

	ctx := mcp.WithCallerUserID(context.Background(), 42)
	_, err := tool.Run(ctx, map[string]interface{}{"tier": "default"})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestSetToolRejectsInvalidModel(t *testing.T) {
	resolver, store := newResolver()
	tool := NewSetTool(resolver, 42, nil)

	ctx := mcp.WithCallerUserID(context.Background(), 42)
	_, err := tool.Run(ctx, map[string]interface{}{"tier": "default", "model": "bad/nope"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if len(store.data) != 0 {
		t.Errorf("store should not be mutated: %v", store.data)
	}
}

func TestGetToolListsAll(t *testing.T) {
	resolver, _ := newResolver()
	_ = resolver.SetOverride(context.Background(), llm.ModelTierStrong, "kr/custom-strong", 42)
	tool := NewGetTool(resolver)

	out, err := tool.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"default", "strong", "router", "kr/custom-strong", "(override)", "fallback-strong"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestResetToolRevertsOverride(t *testing.T) {
	resolver, store := newResolver()
	_ = resolver.SetOverride(context.Background(), llm.ModelTierStrong, "kr/custom-strong", 42)

	audit := &fakeAudit{}
	tool := NewResetTool(resolver, 42, audit)

	ctx := mcp.WithCallerUserID(context.Background(), 42)
	out, err := tool.Run(ctx, map[string]interface{}{"tier": "strong"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "fallback-strong") {
		t.Errorf("response missing fallback: %q", out)
	}
	if got := resolver.Strong(); got != "fallback-strong" {
		t.Errorf("Strong = %q", got)
	}
	if _, ok := store.data[llm.SettingKeyModelStrong]; ok {
		t.Errorf("store key not deleted")
	}
	if len(audit.events) != 1 || audit.events[0].eventType != "model_reset" {
		t.Errorf("audit: %+v", audit.events)
	}
}

func TestResetToolRejectsNonOwner(t *testing.T) {
	resolver, _ := newResolver()
	tool := NewResetTool(resolver, 42, nil)

	ctx := mcp.WithCallerUserID(context.Background(), 999)
	_, err := tool.Run(ctx, map[string]interface{}{"tier": "default"})
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected owner gate error, got: %v", err)
	}
}
