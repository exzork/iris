package reminder

import (
	"context"
	"sync"
	"time"
)

type Store interface {
	Create(ctx context.Context, r *Reminder) (int64, error)
	Get(ctx context.Context, id int64) (*Reminder, error)
	List(ctx context.Context, guildID int64) ([]*Reminder, error)
	Delete(ctx context.Context, guildID, id int64) error
	UpdateNextRun(ctx context.Context, id int64, next time.Time) error
	Due(ctx context.Context, now time.Time) ([]*Reminder, error)
}

type InMemoryStore struct {
	mu      sync.Mutex
	reminders map[int64]*Reminder
	nextID  int64
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		reminders: make(map[int64]*Reminder),
		nextID:    1,
	}
}

func (s *InMemoryStore) Create(ctx context.Context, r *Reminder) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r.ID = s.nextID
	s.nextID++
	r.CreatedAt = time.Now().UTC()

	s.reminders[r.ID] = r
	return r.ID, nil
}

func (s *InMemoryStore) Get(ctx context.Context, id int64) (*Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reminders[id]
	if !ok {
		return nil, ErrNotFound
	}
	return r, nil
}

func (s *InMemoryStore) List(ctx context.Context, guildID int64) ([]*Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*Reminder
	for _, r := range s.reminders {
		if r.GuildID == guildID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (s *InMemoryStore) Delete(ctx context.Context, guildID, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reminders[id]
	if !ok {
		return ErrNotFound
	}
	if r.GuildID != guildID {
		return ErrNotFound
	}

	delete(s.reminders, id)
	return nil
}

func (s *InMemoryStore) UpdateNextRun(ctx context.Context, id int64, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reminders[id]
	if !ok {
		return ErrNotFound
	}

	r.NextRun = next
	return nil
}

func (s *InMemoryStore) Due(ctx context.Context, now time.Time) ([]*Reminder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*Reminder
	for _, r := range s.reminders {
		if !r.NextRun.IsZero() && r.NextRun.Before(now) || r.NextRun.Equal(now) {
			result = append(result, r)
		}
	}
	return result, nil
}
