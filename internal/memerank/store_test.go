package memerank

import (
	"context"
	"testing"
)

func TestInMemoryStoreAddMemeIdempotent(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	meme := &Meme{
		GuildID:    1,
		MessageID:  100,
		ChannelID:  10,
		URL:        "http://example.com/meme.jpg",
		Caption:    "test meme",
		UploaderID: 42,
		Unsafe:     false,
	}

	id1, err := store.AddMeme(ctx, meme)
	if err != nil {
		t.Fatalf("first AddMeme failed: %v", err)
	}

	id2, err := store.AddMeme(ctx, meme)
	if err != nil {
		t.Fatalf("second AddMeme failed: %v", err)
	}

	if id1 != id2 {
		t.Errorf("expected same ID on idempotent add, got %d then %d", id1, id2)
	}

	m, err := store.GetMeme(ctx, id1)
	if err != nil {
		t.Fatalf("GetMeme failed: %v", err)
	}
	if m == nil {
		t.Fatal("expected meme, got nil")
	}
	if m.ID != id1 {
		t.Errorf("expected ID %d, got %d", id1, m.ID)
	}
}

func TestInMemoryStoreUpsertReactionIdempotent(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	rxn := &Reaction{
		GuildID: 1,
		MemeID:  1,
		UserID:  42,
		Kind:    KindUp,
	}

	err := store.UpsertReaction(ctx, rxn)
	if err != nil {
		t.Fatalf("first UpsertReaction failed: %v", err)
	}

	err = store.UpsertReaction(ctx, rxn)
	if err != nil {
		t.Fatalf("second UpsertReaction failed: %v", err)
	}

	up, down, err := store.CountReactions(ctx, 1)
	if err != nil {
		t.Fatalf("CountReactions failed: %v", err)
	}

	if up != 1 {
		t.Errorf("expected 1 up reaction, got %d", up)
	}
	if down != 0 {
		t.Errorf("expected 0 down reactions, got %d", down)
	}
}

func TestInMemoryStoreCountReactions(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()

	rxn1 := &Reaction{
		GuildID: 1,
		MemeID:  1,
		UserID:  42,
		Kind:    KindUp,
	}
	rxn2 := &Reaction{
		GuildID: 1,
		MemeID:  1,
		UserID:  43,
		Kind:    KindUp,
	}
	rxn3 := &Reaction{
		GuildID: 1,
		MemeID:  1,
		UserID:  44,
		Kind:    KindDown,
	}

	store.UpsertReaction(ctx, rxn1)
	store.UpsertReaction(ctx, rxn2)
	store.UpsertReaction(ctx, rxn3)

	up, down, err := store.CountReactions(ctx, 1)
	if err != nil {
		t.Fatalf("CountReactions failed: %v", err)
	}

	if up != 2 {
		t.Errorf("expected 2 up reactions, got %d", up)
	}
	if down != 1 {
		t.Errorf("expected 1 down reaction, got %d", down)
	}
}
