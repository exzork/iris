package memerank

import (
	"context"
	"testing"
)

func TestTopMemesRankingAfterUpvote(t *testing.T) {
	store := NewInMemoryStore()
	svc := NewService(store)
	ctx := context.Background()

	memeA, err := svc.AddMeme(ctx, AddMemeInput{
		GuildID:    1,
		MessageID:  100,
		ChannelID:  10,
		URL:        "http://example.com/a.jpg",
		Caption:    "meme A",
		UploaderID: 42,
		Unsafe:     false,
	})
	if err != nil {
		t.Fatalf("AddMeme A failed: %v", err)
	}

	memeB, err := svc.AddMeme(ctx, AddMemeInput{
		GuildID:    1,
		MessageID:  101,
		ChannelID:  10,
		URL:        "http://example.com/b.jpg",
		Caption:    "meme B",
		UploaderID: 42,
		Unsafe:     false,
	})
	if err != nil {
		t.Fatalf("AddMeme B failed: %v", err)
	}

	err = svc.RecordReaction(ctx, 1, memeB.ID, 42, KindUp)
	if err != nil {
		t.Fatalf("RecordReaction failed: %v", err)
	}

	top, err := svc.TopMemes(ctx, 1, 10)
	if err != nil {
		t.Fatalf("TopMemes failed: %v", err)
	}

	if len(top) != 2 {
		t.Errorf("expected 2 memes, got %d", len(top))
	}

	if top[0].ID != memeB.ID {
		t.Errorf("expected meme B first, got ID %d", top[0].ID)
	}
	if top[1].ID != memeA.ID {
		t.Errorf("expected meme A second, got ID %d", top[1].ID)
	}
}

func TestRecordReactionIdempotent(t *testing.T) {
	store := NewInMemoryStore()
	svc := NewService(store)
	ctx := context.Background()

	meme, err := svc.AddMeme(ctx, AddMemeInput{
		GuildID:    1,
		MessageID:  100,
		ChannelID:  10,
		URL:        "http://example.com/meme.jpg",
		Caption:    "test meme",
		UploaderID: 42,
		Unsafe:     false,
	})
	if err != nil {
		t.Fatalf("AddMeme failed: %v", err)
	}

	err = svc.RecordReaction(ctx, 1, meme.ID, 42, KindUp)
	if err != nil {
		t.Fatalf("first RecordReaction failed: %v", err)
	}

	err = svc.RecordReaction(ctx, 1, meme.ID, 42, KindUp)
	if err != nil {
		t.Fatalf("second RecordReaction failed: %v", err)
	}

	updated, err := store.GetMeme(ctx, meme.ID)
	if err != nil {
		t.Fatalf("GetMeme failed: %v", err)
	}

	if updated.Score != 1 {
		t.Errorf("expected score 1, got %d", updated.Score)
	}
}

func TestGuildIsolation(t *testing.T) {
	store := NewInMemoryStore()
	svc := NewService(store)
	ctx := context.Background()

	meme1, err := svc.AddMeme(ctx, AddMemeInput{
		GuildID:    1,
		MessageID:  100,
		ChannelID:  10,
		URL:        "http://example.com/meme1.jpg",
		Caption:    "guild 1 meme",
		UploaderID: 42,
		Unsafe:     false,
	})
	if err != nil {
		t.Fatalf("AddMeme guild 1 failed: %v", err)
	}

	meme2, err := svc.AddMeme(ctx, AddMemeInput{
		GuildID:    2,
		MessageID:  100,
		ChannelID:  20,
		URL:        "http://example.com/meme2.jpg",
		Caption:    "guild 2 meme",
		UploaderID: 43,
		Unsafe:     false,
	})
	if err != nil {
		t.Fatalf("AddMeme guild 2 failed: %v", err)
	}

	err = svc.RecordReaction(ctx, 1, meme1.ID, 42, KindUp)
	if err != nil {
		t.Fatalf("RecordReaction guild 1 failed: %v", err)
	}

	top1, err := svc.TopMemes(ctx, 1, 10)
	if err != nil {
		t.Fatalf("TopMemes guild 1 failed: %v", err)
	}

	top2, err := svc.TopMemes(ctx, 2, 10)
	if err != nil {
		t.Fatalf("TopMemes guild 2 failed: %v", err)
	}

	if len(top1) != 1 {
		t.Errorf("expected 1 meme in guild 1, got %d", len(top1))
	}
	if top1[0].ID != meme1.ID {
		t.Errorf("expected meme1 in guild 1, got ID %d", top1[0].ID)
	}

	if len(top2) != 1 {
		t.Errorf("expected 1 meme in guild 2, got %d", len(top2))
	}
	if top2[0].ID != meme2.ID {
		t.Errorf("expected meme2 in guild 2, got ID %d", top2[0].ID)
	}
}

func TestUnsafeMemesExcludedFromTop(t *testing.T) {
	store := NewInMemoryStore()
	svc := NewService(store)
	ctx := context.Background()

	safe, err := svc.AddMeme(ctx, AddMemeInput{
		GuildID:    1,
		MessageID:  100,
		ChannelID:  10,
		URL:        "http://example.com/safe.jpg",
		Caption:    "safe meme",
		UploaderID: 42,
		Unsafe:     false,
	})
	if err != nil {
		t.Fatalf("AddMeme safe failed: %v", err)
	}

	unsafe, err := svc.AddMeme(ctx, AddMemeInput{
		GuildID:    1,
		MessageID:  101,
		ChannelID:  10,
		URL:        "http://example.com/unsafe.jpg",
		Caption:    "unsafe meme",
		UploaderID: 42,
		Unsafe:     true,
	})
	if err != nil {
		t.Fatalf("AddMeme unsafe failed: %v", err)
	}

	err = svc.RecordReaction(ctx, 1, unsafe.ID, 42, KindUp)
	if err != nil {
		t.Fatalf("RecordReaction unsafe failed: %v", err)
	}

	top, err := svc.TopMemes(ctx, 1, 10)
	if err != nil {
		t.Fatalf("TopMemes failed: %v", err)
	}

	if len(top) != 1 {
		t.Errorf("expected 1 meme (safe only), got %d", len(top))
	}
	if top[0].ID != safe.ID {
		t.Errorf("expected safe meme, got ID %d", top[0].ID)
	}
}

func TestSwitchingReactionDirection(t *testing.T) {
	store := NewInMemoryStore()
	svc := NewService(store)
	ctx := context.Background()

	meme, err := svc.AddMeme(ctx, AddMemeInput{
		GuildID:    1,
		MessageID:  100,
		ChannelID:  10,
		URL:        "http://example.com/meme.jpg",
		Caption:    "test meme",
		UploaderID: 42,
		Unsafe:     false,
	})
	if err != nil {
		t.Fatalf("AddMeme failed: %v", err)
	}

	err = svc.RecordReaction(ctx, 1, meme.ID, 42, KindUp)
	if err != nil {
		t.Fatalf("first RecordReaction failed: %v", err)
	}

	err = svc.RecordReaction(ctx, 1, meme.ID, 42, KindDown)
	if err != nil {
		t.Fatalf("second RecordReaction failed: %v", err)
	}

	updated, err := store.GetMeme(ctx, meme.ID)
	if err != nil {
		t.Fatalf("GetMeme failed: %v", err)
	}

	if updated.Score != -1 {
		t.Errorf("expected score -1, got %d", updated.Score)
	}
}

func TestTopMemesLimitRespected(t *testing.T) {
	store := NewInMemoryStore()
	svc := NewService(store)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := svc.AddMeme(ctx, AddMemeInput{
			GuildID:    1,
			MessageID:  int64(100 + i),
			ChannelID:  10,
			URL:        "http://example.com/meme.jpg",
			Caption:    "test meme",
			UploaderID: 42,
			Unsafe:     false,
		})
		if err != nil {
			t.Fatalf("AddMeme failed: %v", err)
		}
	}

	top, err := svc.TopMemes(ctx, 1, 3)
	if err != nil {
		t.Fatalf("TopMemes failed: %v", err)
	}

	if len(top) != 3 {
		t.Errorf("expected 3 memes with limit 3, got %d", len(top))
	}
}
