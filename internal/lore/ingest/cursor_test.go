package ingest

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryCursorStoreSaveGet(t *testing.T) {
	store := NewInMemoryCursorStore()
	in := &Cursor{SourceID: "wiki-main", LastID: 12, LastTitle: "Page 12", UpdatedAt: time.Now().UTC()}
	if err := store.Save(context.Background(), in); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Get(context.Background(), "wiki-main")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatalf("expected cursor, got nil")
	}
	if got.LastID != in.LastID || got.LastTitle != in.LastTitle || got.SourceID != in.SourceID {
		t.Fatalf("unexpected cursor: %+v", got)
	}
}

func TestInMemoryCursorStoreGetMissingReturnsNil(t *testing.T) {
	store := NewInMemoryCursorStore()
	got, err := store.Get(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil cursor for missing key, got %+v", got)
	}
}
