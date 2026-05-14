package repository

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestLoreSessionRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	repo := NewLoreSessionRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 888888888, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("OpenOrRefreshCreatesSession", func(t *testing.T) {
		guildID := int64(888888888)
		channelID := int64(111111111)
		msgID := int64(555555555)
		msgTime := time.Now()
		idleDeadline := msgTime.Add(5 * time.Minute)

		sessionID, err := repo.OpenOrRefresh(ctx, guildID, channelID, msgID, msgTime, idleDeadline)
		if err != nil {
			t.Fatalf("OpenOrRefresh failed: %v", err)
		}
		if sessionID == 0 {
			t.Error("expected non-zero sessionID")
		}
	})

	t.Run("GetOpenByChannelReturnsSession", func(t *testing.T) {
		guildID := int64(888888888)
		channelID := int64(222222222)
		msgID := int64(666666666)
		msgTime := time.Now()
		idleDeadline := msgTime.Add(5 * time.Minute)

		sessionID, err := repo.OpenOrRefresh(ctx, guildID, channelID, msgID, msgTime, idleDeadline)
		if err != nil {
			t.Fatalf("OpenOrRefresh failed: %v", err)
		}

		session, err := repo.GetOpenByChannel(ctx, guildID, channelID)
		if err != nil {
			t.Fatalf("GetOpenByChannel failed: %v", err)
		}
		if session.ID != sessionID {
			t.Errorf("expected session ID %d, got %d", sessionID, session.ID)
		}
		if session.Status != "open" {
			t.Errorf("expected status 'open', got %q", session.Status)
		}
	})

	t.Run("MarkStatusUpdatesStatus", func(t *testing.T) {
		guildID := int64(888888888)
		channelID := int64(333333333)
		msgID := int64(777777777)
		msgTime := time.Now()
		idleDeadline := msgTime.Add(5 * time.Minute)

		sessionID, err := repo.OpenOrRefresh(ctx, guildID, channelID, msgID, msgTime, idleDeadline)
		if err != nil {
			t.Fatalf("OpenOrRefresh failed: %v", err)
		}

		err = repo.MarkStatus(ctx, sessionID, "summarizing")
		if err != nil {
			t.Fatalf("MarkStatus failed: %v", err)
		}

		_, err = repo.GetOpenByChannel(ctx, guildID, channelID)
		if err == nil {
			t.Error("expected GetOpenByChannel to fail after status changed to summarizing")
		}
	})

	t.Run("SetThreadResultUpdatesThread", func(t *testing.T) {
		guildID := int64(888888888)
		channelID := int64(444444444)
		msgID := int64(888888888)
		msgTime := time.Now()
		idleDeadline := msgTime.Add(5 * time.Minute)

		sessionID, err := repo.OpenOrRefresh(ctx, guildID, channelID, msgID, msgTime, idleDeadline)
		if err != nil {
			t.Fatalf("OpenOrRefresh failed: %v", err)
		}

		threadID := int64(999999999)
		summaryMsgID := int64(111111112)
		title := "Test Lore Thread"
		summary := "This is a test summary"

		err = repo.SetThreadResult(ctx, sessionID, threadID, summaryMsgID, title, summary)
		if err != nil {
			t.Fatalf("SetThreadResult failed: %v", err)
		}
	})

	t.Run("IncrementRetryIncrementsCount", func(t *testing.T) {
		guildID := int64(888888888)
		channelID := int64(555555555)
		msgID := int64(222222222)
		msgTime := time.Now()
		idleDeadline := msgTime.Add(5 * time.Minute)

		sessionID, err := repo.OpenOrRefresh(ctx, guildID, channelID, msgID, msgTime, idleDeadline)
		if err != nil {
			t.Fatalf("OpenOrRefresh failed: %v", err)
		}

		err = repo.IncrementRetry(ctx, sessionID, "test error")
		if err != nil {
			t.Fatalf("IncrementRetry failed: %v", err)
		}
	})
}
