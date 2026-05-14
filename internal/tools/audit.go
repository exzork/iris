package tools

import (
	"context"
	"sync"
	"time"
)

type AuditEvent struct {
	GuildID  int64
	UserID   int64
	Tool     string
	Status   string
	Duration time.Duration
	Error    string
	At       time.Time
}

type AuditLogger interface {
	Record(ctx context.Context, evt AuditEvent) error
}

type InMemoryAudit struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (a *InMemoryAudit) Record(ctx context.Context, evt AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, evt)
	return nil
}

func (a *InMemoryAudit) Events() []AuditEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]AuditEvent, len(a.events))
	copy(result, a.events)
	return result
}
