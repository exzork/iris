package memerank

import (
	"context"
	"sync"
	"time"
)

type Store interface {
	AddMeme(ctx context.Context, m *Meme) (int64, error)
	GetMeme(ctx context.Context, id int64) (*Meme, error)
	ListByGuild(ctx context.Context, guildID int64, limit int) ([]*Meme, error)
	UpsertReaction(ctx context.Context, r *Reaction) error
	CountReactions(ctx context.Context, memeID int64) (up int, down int, err error)
}

type InMemoryStore struct {
	mu         sync.Mutex
	memes      map[int64]*Meme
	memeByKey  map[string]int64
	reactions  map[string]*Reaction
	nextMemeID int64
	nextRxnID  int64
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		memes:      make(map[int64]*Meme),
		memeByKey:  make(map[string]int64),
		reactions:  make(map[string]*Reaction),
		nextMemeID: 1,
		nextRxnID:  1,
	}
}

func (s *InMemoryStore) AddMeme(ctx context.Context, m *Meme) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := memeKey(m.GuildID, m.MessageID)
	if id, exists := s.memeByKey[key]; exists {
		return id, nil
	}

	m.ID = s.nextMemeID
	s.nextMemeID++
	m.CreatedAt = time.Now()

	s.memes[m.ID] = m
	s.memeByKey[key] = m.ID

	return m.ID, nil
}

func (s *InMemoryStore) GetMeme(ctx context.Context, id int64) (*Meme, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.memes[id]
	if !ok {
		return nil, nil
	}
	return m, nil
}

func (s *InMemoryStore) ListByGuild(ctx context.Context, guildID int64, limit int) ([]*Meme, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*Meme
	for _, m := range s.memes {
		if m.GuildID == guildID {
			result = append(result, m)
		}
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *InMemoryStore) UpsertReaction(ctx context.Context, r *Reaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := reactionKey(r.MemeID, r.UserID)
	if existing, ok := s.reactions[key]; ok {
		existing.Kind = r.Kind
		existing.CreatedAt = time.Now()
	} else {
		r.ID = s.nextRxnID
		s.nextRxnID++
		r.CreatedAt = time.Now()
		s.reactions[key] = r
	}
	return nil
}

func (s *InMemoryStore) CountReactions(ctx context.Context, memeID int64) (up int, down int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range s.reactions {
		if r.MemeID == memeID {
			if r.Kind == KindUp {
				up++
			} else if r.Kind == KindDown {
				down++
			}
		}
	}
	return up, down, nil
}

func memeKey(guildID, messageID int64) string {
	return "guild:" + string(rune(guildID)) + ":msg:" + string(rune(messageID))
}

func reactionKey(memeID, userID int64) string {
	return "meme:" + string(rune(memeID)) + ":user:" + string(rune(userID))
}
