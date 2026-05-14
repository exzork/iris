package tools

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestInMemoryAuditAppendAndList(t *testing.T) {
	audit := &InMemoryAudit{}
	ctx := context.Background()

	evt1 := AuditEvent{
		GuildID:  123,
		UserID:   456,
		Tool:     "search",
		Status:   "ok",
		Duration: 100 * time.Millisecond,
		At:       time.Now(),
	}
	evt2 := AuditEvent{
		GuildID:  123,
		UserID:   789,
		Tool:     "fetch",
		Status:   "error",
		Duration: 50 * time.Millisecond,
		Error:    "network timeout",
		At:       time.Now(),
	}

	if err := audit.Record(ctx, evt1); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if err := audit.Record(ctx, evt2); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	events := audit.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].Tool != "search" {
		t.Fatalf("expected first tool to be search, got %s", events[0].Tool)
	}
	if events[1].Tool != "fetch" {
		t.Fatalf("expected second tool to be fetch, got %s", events[1].Tool)
	}
}

func TestInMemoryAuditConcurrent(t *testing.T) {
	audit := &InMemoryAudit{}
	ctx := context.Background()
	numGoroutines := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			evt := AuditEvent{
				GuildID:  int64(idx),
				UserID:   int64(idx * 2),
				Tool:     "concurrent_tool",
				Status:   "ok",
				Duration: time.Millisecond,
				At:       time.Now(),
			}
			if err := audit.Record(ctx, evt); err != nil {
				t.Errorf("Record failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	events := audit.Events()
	if len(events) != numGoroutines {
		t.Fatalf("expected %d events, got %d", numGoroutines, len(events))
	}
}
