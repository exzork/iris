package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

type Deduper interface {
	SeenHash(ctx context.Context, hash string) (bool, error)
	MarkHash(ctx context.Context, hash string) error
}

type InMemoryDeduper struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewInMemoryDeduper() *InMemoryDeduper {
	return &InMemoryDeduper{seen: make(map[string]struct{})}
}

func (d *InMemoryDeduper) SeenHash(_ context.Context, hash string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.seen[hash]
	return ok, nil
}

func (d *InMemoryDeduper) MarkHash(_ context.Context, hash string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen[hash] = struct{}{}
	return nil
}

func ContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
