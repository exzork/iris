package charlookup

import (
	"context"
	"fmt"
)

type CharacterStore interface {
	GetByID(ctx context.Context, id int64) (*Character, error)
	List(ctx context.Context) ([]*Character, error)
}

type InMemoryStore struct {
	characters map[int64]*Character
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		characters: make(map[int64]*Character),
	}
}

func (s *InMemoryStore) Add(char *Character) error {
	if char == nil {
		return fmt.Errorf("character cannot be nil")
	}
	if char.ID == 0 {
		return fmt.Errorf("character ID must be non-zero")
	}
	s.characters[char.ID] = char
	return nil
}

func (s *InMemoryStore) GetByID(ctx context.Context, id int64) (*Character, error) {
	char, ok := s.characters[id]
	if !ok {
		return nil, fmt.Errorf("character not found: id=%d", id)
	}
	return char, nil
}

func (s *InMemoryStore) List(ctx context.Context) ([]*Character, error) {
	result := make([]*Character, 0, len(s.characters))
	for _, char := range s.characters {
		result = append(result, char)
	}
	return result, nil
}
