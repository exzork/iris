package memerank

import (
	"context"
	"sort"
	"time"
)

type Service struct {
	Store Store
}

func NewService(store Store) *Service {
	return &Service{Store: store}
}

type AddMemeInput struct {
	GuildID    int64
	MessageID  int64
	ChannelID  int64
	URL        string
	Caption    string
	UploaderID int64
	Unsafe     bool
}

func (s *Service) AddMeme(ctx context.Context, input AddMemeInput) (*Meme, error) {
	meme := &Meme{
		GuildID:    input.GuildID,
		MessageID:  input.MessageID,
		ChannelID:  input.ChannelID,
		URL:        input.URL,
		Caption:    input.Caption,
		UploaderID: input.UploaderID,
		Unsafe:     input.Unsafe,
		Score:      0,
		CreatedAt:  time.Now(),
	}

	id, err := s.Store.AddMeme(ctx, meme)
	if err != nil {
		return nil, err
	}

	meme.ID = id
	return meme, nil
}

func (s *Service) RecordReaction(ctx context.Context, guildID, memeID, userID int64, kind ReactionKind) error {
	meme, err := s.Store.GetMeme(ctx, memeID)
	if err != nil {
		return err
	}
	if meme == nil {
		return nil
	}

	if meme.GuildID != guildID {
		return nil
	}

	rxn := &Reaction{
		GuildID:   guildID,
		MemeID:    memeID,
		UserID:    userID,
		Kind:      kind,
		CreatedAt: time.Now(),
	}

	err = s.Store.UpsertReaction(ctx, rxn)
	if err != nil {
		return err
	}

	up, down, err := s.Store.CountReactions(ctx, memeID)
	if err != nil {
		return err
	}

	meme.Score = up - down

	_, err = s.Store.AddMeme(ctx, meme)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) TopMemes(ctx context.Context, guildID int64, n int) ([]*Meme, error) {
	memes, err := s.Store.ListByGuild(ctx, guildID, 0)
	if err != nil {
		return nil, err
	}

	var filtered []*Meme
	for _, m := range memes {
		if !m.Unsafe {
			filtered = append(filtered, m)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	if n > 0 && len(filtered) > n {
		filtered = filtered[:n]
	}

	return filtered, nil
}
