package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestUserBehaviorRepo_UpsertRequiresGuild(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewUserBehaviorRepo(db)
	err := repo.Upsert(context.Background(), &domain.UserBehaviorProfile{
		UserID:             99,
		CommunicationStyle: "playful",
	})
	if !errors.Is(err, ErrMissingGuildID) {
		t.Fatalf("want ErrMissingGuildID, got %v", err)
	}
}

func TestUserBehaviorRepo_UpsertRequiresUser(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewUserBehaviorRepo(db)
	err := repo.Upsert(context.Background(), &domain.UserBehaviorProfile{
		GuildID: 1,
	})
	if err == nil || err == ErrMissingGuildID {
		t.Fatalf("expected user_id required error, got %v", err)
	}
}

func TestUserBehaviorRepo_RoundtripIsGuildUserScoped(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	ctx := context.Background()
	seedGuildForTest(t, ctx, db, 7001)
	seedGuildForTest(t, ctx, db, 7002)

	repo := NewUserBehaviorRepo(db)

	profileA := &domain.UserBehaviorProfile{
		GuildID:                  7001,
		UserID:                   555,
		CommunicationStyle:       "playful",
		Formality:                "informal",
		ResponseLengthPreference: "concise",
		FormattingPreference:     "markdown",
		RecurringTopics:          []string{"lore", "characters"},
		EvidenceCount:            3,
		LastObservedAt:           time.Now(),
	}
	if err := repo.Upsert(ctx, profileA); err != nil {
		t.Fatalf("upsert A: %v", err)
	}

	profileBSameUser := &domain.UserBehaviorProfile{
		GuildID:                  7002,
		UserID:                   555,
		CommunicationStyle:       "formal",
		Formality:                "formal",
		ResponseLengthPreference: "long",
		FormattingPreference:     "plain",
		RecurringTopics:          []string{"strategy"},
		EvidenceCount:            8,
		LastObservedAt:           time.Now(),
	}
	if err := repo.Upsert(ctx, profileBSameUser); err != nil {
		t.Fatalf("upsert B: %v", err)
	}

	gotA, err := repo.GetByGuildUser(ctx, 7001, 555)
	if err != nil {
		t.Fatalf("get A: %v", err)
	}
	if gotA == nil || gotA.CommunicationStyle != "playful" || gotA.RecurringTopics[0] != "lore" {
		t.Fatalf("guild A profile wrong: %+v", gotA)
	}

	gotB, err := repo.GetByGuildUser(ctx, 7002, 555)
	if err != nil {
		t.Fatalf("get B: %v", err)
	}
	if gotB == nil || gotB.CommunicationStyle != "formal" {
		t.Fatalf("guild B profile wrong: %+v", gotB)
	}

	if gotA.CommunicationStyle == gotB.CommunicationStyle {
		t.Fatalf("same user's profile leaked across guilds")
	}
}

func TestUserBehaviorRepo_GetRejectsMissingGuild(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewUserBehaviorRepo(db)
	_, err := repo.GetByGuildUser(context.Background(), 0, 42)
	if !errors.Is(err, ErrMissingGuildID) {
		t.Fatalf("want ErrMissingGuildID, got %v", err)
	}
}

func TestUserBehaviorRepo_GetRejectsMissingUser(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	repo := NewUserBehaviorRepo(db)
	_, err := repo.GetByGuildUser(context.Background(), 1, 0)
	if err == nil || errors.Is(err, ErrMissingGuildID) {
		t.Fatalf("expected user_id required error, got %v", err)
	}
}
