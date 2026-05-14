package obs

import (
	"context"
	"testing"
)

func TestNewCorrelationIDFormat(t *testing.T) {
	id := NewCorrelationID()
	if len(id) != 32 {
		t.Errorf("expected 32 hex chars, got %d: %s", len(id), id)
	}
	for _, ch := range id {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			t.Errorf("expected lowercase hex, got invalid char: %c", ch)
		}
	}
}

func TestWithCorrelationIDRoundtrip(t *testing.T) {
	ctx := context.Background()
	id := "test-correlation-123"
	ctx = WithCorrelationID(ctx, id)
	retrieved := CorrelationID(ctx)
	if retrieved != id {
		t.Errorf("expected %s, got %s", id, retrieved)
	}
}

func TestEnsureCreatesWhenMissing(t *testing.T) {
	ctx := context.Background()
	newCtx, id := EnsureCorrelationID(ctx)
	if id == "" {
		t.Error("expected non-empty correlation ID")
	}
	if len(id) != 32 {
		t.Errorf("expected 32 hex chars, got %d", len(id))
	}
	retrieved := CorrelationID(newCtx)
	if retrieved != id {
		t.Errorf("expected %s, got %s", id, retrieved)
	}
}

func TestEnsurePreservesExisting(t *testing.T) {
	ctx := context.Background()
	existingID := "existing-id-12345"
	ctx = WithCorrelationID(ctx, existingID)
	newCtx, id := EnsureCorrelationID(ctx)
	if id != existingID {
		t.Errorf("expected %s, got %s", existingID, id)
	}
	if newCtx != ctx {
		t.Error("expected same context when ID already exists")
	}
}
