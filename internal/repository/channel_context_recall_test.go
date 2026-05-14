package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func seedGuildForTest(t *testing.T, ctx context.Context, db *DB, guildID int64) {
	t.Helper()
	guildRepo := NewGuildRepo(db)
	if err := guildRepo.Create(ctx, &domain.Guild{
		ID:        guildID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed guild %d: %v", guildID, err)
	}
}

func makeEmbedding(seed float32) []float32 {
	out := make([]float32, ExpectedEmbeddingDim)
	for i := range out {
		out[i] = seed
	}
	return out
}

func insertChannelMessage(t *testing.T, ctx context.Context, repo *ChannelMessageRepo, msg *domain.ChannelMessage) {
	t.Helper()
	if err := repo.Upsert(ctx, msg); err != nil {
		t.Fatalf("upsert message: %v", err)
	}
}

func TestChannelMessageRepo_StoreEmbedding_RejectsMissingGuild(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewChannelMessageRepo(db)
	err := repo.StoreEmbedding(context.Background(), 0, 1, makeEmbedding(0.1))
	if !errors.Is(err, ErrMissingGuildID) {
		t.Fatalf("want ErrMissingGuildID, got %v", err)
	}
}

func TestChannelMessageRepo_StoreEmbedding_RejectsWrongDim(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewChannelMessageRepo(db)
	err := repo.StoreEmbedding(context.Background(), 1, 1, []float32{0.1, 0.2})
	if !errors.Is(err, ErrInvalidVectorDim) {
		t.Fatalf("want ErrInvalidVectorDim, got %v", err)
	}
}

func TestChannelMessageRepo_RecallByVector_RejectsMissingGuild(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewChannelMessageRepo(db)
	_, err := repo.RecallByVector(context.Background(), 0, makeEmbedding(0.1), 0.5, 5)
	if !errors.Is(err, ErrMissingGuildID) {
		t.Fatalf("want ErrMissingGuildID, got %v", err)
	}
}

func TestChannelMessageRepo_RecallByVector_SameGuildOnly(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	ctx := context.Background()
	seedGuildForTest(t, ctx, db, 1001)
	seedGuildForTest(t, ctx, db, 1002)

	repo := NewChannelMessageRepo(db)

	guildAEmbedding := makeEmbedding(0.3)
	guildBEmbedding := makeEmbedding(0.3)

	insertChannelMessage(t, ctx, repo, &domain.ChannelMessage{
		GuildID:          1001,
		ChannelID:        5001,
		MessageID:        900001,
		UserID:           77,
		Content:          "hello from guild A",
		TriggerSource:    "observe",
		CreatedAt:        time.Now(),
		ContentEmbedding: guildAEmbedding,
	})
	insertChannelMessage(t, ctx, repo, &domain.ChannelMessage{
		GuildID:          1002,
		ChannelID:        5002,
		MessageID:        900002,
		UserID:           77,
		Content:          "hello from guild B",
		TriggerSource:    "observe",
		CreatedAt:        time.Now(),
		ContentEmbedding: guildBEmbedding,
	})

	results, err := repo.RecallByVector(ctx, 1001, makeEmbedding(0.3), 0.5, 5)
	if err != nil {
		t.Fatalf("recall guild A: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected >=1 result for guild A")
	}
	for _, r := range results {
		if r.Message.GuildID != 1001 {
			t.Fatalf("guild A query returned row from guild %d", r.Message.GuildID)
		}
	}

	resultsB, err := repo.RecallByVector(ctx, 1002, makeEmbedding(0.3), 0.5, 5)
	if err != nil {
		t.Fatalf("recall guild B: %v", err)
	}
	for _, r := range resultsB {
		if r.Message.GuildID != 1002 {
			t.Fatalf("guild B query returned row from guild %d", r.Message.GuildID)
		}
	}
}

func TestChannelMessageRepo_RecallByVector_RespectsThreshold(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	ctx := context.Background()
	seedGuildForTest(t, ctx, db, 2001)
	repo := NewChannelMessageRepo(db)

	insertChannelMessage(t, ctx, repo, &domain.ChannelMessage{
		GuildID:          2001,
		ChannelID:        3001,
		MessageID:        910001,
		UserID:           42,
		Content:          "close match",
		TriggerSource:    "observe",
		CreatedAt:        time.Now(),
		ContentEmbedding: makeEmbedding(0.4),
	})
	insertChannelMessage(t, ctx, repo, &domain.ChannelMessage{
		GuildID:          2001,
		ChannelID:        3001,
		MessageID:        910002,
		UserID:           42,
		Content:          "far match",
		TriggerSource:    "observe",
		CreatedAt:        time.Now(),
		ContentEmbedding: makeOppositeEmbedding(),
	})

	results, err := repo.RecallByVector(ctx, 2001, makeEmbedding(0.4), 0.9, 5)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	for _, r := range results {
		if r.Similarity < 0.9 {
			t.Fatalf("threshold violated: %v", r.Similarity)
		}
	}
}

func TestChannelMessageRepo_ListPendingEmbeddings_OnlyNullRows(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	ctx := context.Background()
	seedGuildForTest(t, ctx, db, 3001)
	repo := NewChannelMessageRepo(db)

	insertChannelMessage(t, ctx, repo, &domain.ChannelMessage{
		GuildID:       3001,
		ChannelID:     4001,
		MessageID:     920001,
		UserID:        10,
		Content:       "pending row",
		TriggerSource: "observe",
		CreatedAt:     time.Now(),
	})
	insertChannelMessage(t, ctx, repo, &domain.ChannelMessage{
		GuildID:          3001,
		ChannelID:        4001,
		MessageID:        920002,
		UserID:           10,
		Content:          "already embedded",
		TriggerSource:    "observe",
		CreatedAt:        time.Now(),
		ContentEmbedding: makeEmbedding(0.2),
	})

	pending, err := repo.ListPendingEmbeddings(ctx, 10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	foundPending := false
	for _, p := range pending {
		if p.MessageID == 920001 {
			foundPending = true
		}
		if p.MessageID == 920002 {
			t.Fatalf("already-embedded row returned as pending")
		}
	}
	if !foundPending {
		t.Fatalf("pending row 920001 not returned")
	}
}

func TestChannelMessageRepo_StoreEmbedding_IsScopedByGuild(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	ctx := context.Background()
	seedGuildForTest(t, ctx, db, 4001)
	seedGuildForTest(t, ctx, db, 4002)
	repo := NewChannelMessageRepo(db)

	insertChannelMessage(t, ctx, repo, &domain.ChannelMessage{
		GuildID: 4001, ChannelID: 8001, MessageID: 930001, UserID: 1,
		Content: "guild A", TriggerSource: "observe", CreatedAt: time.Now(),
	})
	insertChannelMessage(t, ctx, repo, &domain.ChannelMessage{
		GuildID: 4002, ChannelID: 8002, MessageID: 930001, UserID: 1,
		Content: "guild B", TriggerSource: "observe", CreatedAt: time.Now(),
	})

	if err := repo.StoreEmbedding(ctx, 4001, 930001, makeEmbedding(0.5)); err != nil {
		t.Fatalf("store A: %v", err)
	}

	pending, err := repo.ListPendingEmbeddings(ctx, 50)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	var guildBStillPending, guildAStillPending bool
	for _, p := range pending {
		if p.GuildID == 4001 && p.MessageID == 930001 {
			guildAStillPending = true
		}
		if p.GuildID == 4002 && p.MessageID == 930001 {
			guildBStillPending = true
		}
	}
	if guildAStillPending {
		t.Fatalf("guild A row should no longer be pending")
	}
	if !guildBStillPending {
		t.Fatalf("guild B row should still be pending (isolation)")
	}
}

func makeOppositeEmbedding() []float32 {
	out := make([]float32, ExpectedEmbeddingDim)
	for i := range out {
		if i%2 == 0 {
			out[i] = -1
		} else {
			out[i] = 1
		}
	}
	return out
}
