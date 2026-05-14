package repository

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestLoreThreadAnchorRepo(t *testing.T) {
	db := setupTestDB(t)
	defer closeTestDB(t, db)

	guildRepo := NewGuildRepo(db)
	sessionRepo := NewLoreSessionRepo(db)
	repo := NewLoreThreadAnchorRepo(db)
	ctx := context.Background()

	guild := &domain.Guild{ID: 777777777, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	guildRepo.Create(ctx, guild)

	t.Run("InsertCreatesAnchor", func(t *testing.T) {
		guildID := int64(777777777)
		channelID := int64(111111111)
		threadID := int64(222222222)
		summaryMsgID := int64(333333333)
		title := "Test Thread"
		summaryText := "Test summary"

		anchor := &domain.LoreThreadAnchor{
			GuildID:          guildID,
			ChannelID:        channelID,
			ThreadID:         threadID,
			SummaryMessageID: &summaryMsgID,
			SummaryText:      &summaryText,
			Title:            &title,
		}

		err := repo.Insert(ctx, anchor)
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	})

	t.Run("GetByThreadReturnsAnchor", func(t *testing.T) {
		guildID := int64(777777777)
		channelID := int64(444444444)
		threadID := int64(555555555)
		summaryMsgID := int64(666666666)
		title := "Another Thread"
		summaryText := "Another summary"

		anchor := &domain.LoreThreadAnchor{
			GuildID:          guildID,
			ChannelID:        channelID,
			ThreadID:         threadID,
			SummaryMessageID: &summaryMsgID,
			SummaryText:      &summaryText,
			Title:            &title,
		}

		err := repo.Insert(ctx, anchor)
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}

		retrieved, err := repo.GetByThread(ctx, guildID, threadID)
		if err != nil {
			t.Fatalf("GetByThread failed: %v", err)
		}
		if retrieved.ThreadID != threadID {
			t.Errorf("expected thread ID %d, got %d", threadID, retrieved.ThreadID)
		}
		if retrieved.Title == nil || *retrieved.Title != title {
			t.Errorf("expected title %q, got %v", title, retrieved.Title)
		}
	})

	t.Run("InsertWithSourceSessionID", func(t *testing.T) {
		guildID := int64(777777777)
		channelID := int64(777777777)
		msgID := int64(888888888)
		msgTime := time.Now()
		idleDeadline := msgTime.Add(5 * time.Minute)

		sessionID, err := sessionRepo.OpenOrRefresh(ctx, guildID, channelID, msgID, msgTime, idleDeadline)
		if err != nil {
			t.Fatalf("OpenOrRefresh failed: %v", err)
		}

		threadID := int64(999999999)
		summaryMsgID := int64(111111113)
		title := "Thread with Session"
		summaryText := "Summary with session"

		anchor := &domain.LoreThreadAnchor{
			GuildID:          guildID,
			ChannelID:        channelID,
			ThreadID:         threadID,
			SummaryMessageID: &summaryMsgID,
			SummaryText:      &summaryText,
			Title:            &title,
			SourceSessionID:  &sessionID,
		}

		err = repo.Insert(ctx, anchor)
		if err != nil {
			t.Fatalf("Insert with source session failed: %v", err)
		}

		retrieved, err := repo.GetByThread(ctx, guildID, threadID)
		if err != nil {
			t.Fatalf("GetByThread failed: %v", err)
		}
		if retrieved.SourceSessionID == nil || *retrieved.SourceSessionID != sessionID {
			t.Errorf("expected source session ID %d, got %v", sessionID, retrieved.SourceSessionID)
		}
	})
}
