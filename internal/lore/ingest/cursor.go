package ingest

import (
	"context"
	"sync"
	"time"
)

type Cursor struct {
	SourceID  string
	LastID    int64
	LastTitle string
	Continue  string
	UpdatedAt time.Time
}

type CursorStore interface {
	Get(ctx context.Context, sourceID string) (*Cursor, error)
	Save(ctx context.Context, cur *Cursor) error
}

// InMemoryCursorStore is safe for concurrent use.
type InMemoryCursorStore struct {
	mu    sync.Mutex
	items map[string]*Cursor
}

func NewInMemoryCursorStore() *InMemoryCursorStore {
	return &InMemoryCursorStore{items: make(map[string]*Cursor)}
}

func (s *InMemoryCursorStore) Get(_ context.Context, sourceID string) (*Cursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.items[sourceID]
	if !ok {
		return nil, nil
	}
	copyCur := *cur
	return &copyCur, nil
}

func (s *InMemoryCursorStore) Save(_ context.Context, cur *Cursor) error {
	if cur == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copyCur := *cur
	if copyCur.UpdatedAt.IsZero() {
		copyCur.UpdatedAt = time.Now().UTC()
	}
	s.items[cur.SourceID] = &copyCur
	return nil
}
