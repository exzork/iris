package itemlookup

import (
	"context"
	"strings"
	"sync"
)

type ItemStore interface {
	GetByID(ctx context.Context, id int64) (*Item, error)
	FindByAlias(ctx context.Context, alias string) ([]*Item, error)
	List(ctx context.Context) ([]*Item, error)
}

type InMemoryStore struct {
	mu       sync.RWMutex
	items    map[int64]*Item
	aliases  map[string][]*Item
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		items:   make(map[int64]*Item),
		aliases: make(map[string][]*Item),
	}
}

func (s *InMemoryStore) Add(item *Item) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[item.ID] = item

	lowerName := strings.ToLower(item.Name)
	s.aliases[lowerName] = append(s.aliases[lowerName], item)

	for _, alias := range item.Aliases {
		lowerAlias := strings.ToLower(alias)
		s.aliases[lowerAlias] = append(s.aliases[lowerAlias], item)
	}
}

func (s *InMemoryStore) GetByID(ctx context.Context, id int64) (*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.items[id]
	if !ok {
		return nil, nil
	}
	return item, nil
}

func (s *InMemoryStore) FindByAlias(ctx context.Context, alias string) ([]*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lowerAlias := strings.ToLower(strings.TrimSpace(alias))
	items, ok := s.aliases[lowerAlias]
	if !ok {
		return []*Item{}, nil
	}

	result := make([]*Item, len(items))
	copy(result, items)
	return result, nil
}

func (s *InMemoryStore) List(ctx context.Context) ([]*Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Item, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	return result, nil
}
