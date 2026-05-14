package repository

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestChannelConversationRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	repo := NewChannelConversationRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 999999999, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("RefreshCreatesAndExtends", func(t *testing.T) {
		guildID := int64(999999999)
		channelID := int64(111111111)
		now := time.Now()
		ttl := 5 * time.Second

		err := repo.Refresh(ctx, guildID, channelID, now, ttl)
		if err != nil {
			t.Fatalf("first Refresh failed: %v", err)
		}

		active, err := repo.Active(ctx, guildID, channelID, now)
		if err != nil {
			t.Fatalf("Active check failed: %v", err)
		}
		if !active {
			t.Error("expected Active to return true after Refresh")
		}

		laterNow := now.Add(2 * time.Second)
		err = repo.Refresh(ctx, guildID, channelID, laterNow, ttl)
		if err != nil {
			t.Fatalf("second Refresh failed: %v", err)
		}

		active, err = repo.Active(ctx, guildID, channelID, laterNow)
		if err != nil {
			t.Fatalf("Active check after second Refresh failed: %v", err)
		}
		if !active {
			t.Error("expected Active to return true after second Refresh")
		}
	})

	t.Run("ActiveExpiresAfterTTL", func(t *testing.T) {
		guildID := int64(999999999)
		channelID := int64(222222222)
		now := time.Now()
		ttl := 1 * time.Second

		err := repo.Refresh(ctx, guildID, channelID, now, ttl)
		if err != nil {
			t.Fatalf("Refresh failed: %v", err)
		}

		active, err := repo.Active(ctx, guildID, channelID, now)
		if err != nil {
			t.Fatalf("Active check before expiry failed: %v", err)
		}
		if !active {
			t.Error("expected Active to return true before lock_until")
		}

		afterExpiry := now.Add(2 * time.Second)
		active, err = repo.Active(ctx, guildID, channelID, afterExpiry)
		if err != nil {
			t.Fatalf("Active check after expiry failed: %v", err)
		}
		if active {
			t.Error("expected Active to return false after lock_until")
		}
	})

	t.Run("ClearRemovesRow", func(t *testing.T) {
		guildID := int64(999999999)
		channelID := int64(333333333)
		now := time.Now()
		ttl := 5 * time.Second

		err := repo.Refresh(ctx, guildID, channelID, now, ttl)
		if err != nil {
			t.Fatalf("Refresh failed: %v", err)
		}

		active, err := repo.Active(ctx, guildID, channelID, now)
		if err != nil {
			t.Fatalf("Active check before Clear failed: %v", err)
		}
		if !active {
			t.Error("expected Active to return true before Clear")
		}

		err = repo.Clear(ctx, guildID, channelID)
		if err != nil {
			t.Fatalf("Clear failed: %v", err)
		}

		active, err = repo.Active(ctx, guildID, channelID, now)
		if err != nil {
			t.Fatalf("Active check after Clear failed: %v", err)
		}
		if active {
			t.Error("expected Active to return false after Clear")
		}
	})

	t.Run("FKCascade", func(t *testing.T) {
		cascadeGuild := &domain.Guild{ID: 888888888, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		guildRepo.Create(ctx, cascadeGuild)

		guildID := int64(888888888)
		channelID := int64(444444444)
		now := time.Now()
		ttl := 5 * time.Second

		err := repo.Refresh(ctx, guildID, channelID, now, ttl)
		if err != nil {
			t.Fatalf("Refresh failed: %v", err)
		}

		active, err := repo.Active(ctx, guildID, channelID, now)
		if err != nil {
			t.Fatalf("Active check before guild delete failed: %v", err)
		}
		if !active {
			t.Error("expected Active to return true before guild delete")
		}

		err = guildRepo.Delete(ctx, guildID)
		if err != nil {
			t.Fatalf("guild Delete failed: %v", err)
		}

		active, err = repo.Active(ctx, guildID, channelID, now)
		if err != nil {
			t.Fatalf("Active check after guild delete failed: %v", err)
		}
		if active {
			t.Error("expected Active to return false after guild cascade delete")
		}
	})
}
