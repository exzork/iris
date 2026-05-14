package wire

import (
	"context"
	"fmt"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/lorethread"
	"github.com/eko/iris-bot/internal/repository"
)

// LoreThreadAnchorStoreAdapter adapts repository anchor/session repos to lorethread.ThreadAnchorStore.
type LoreThreadAnchorStoreAdapter struct {
	anchorRepo  *repository.LoreThreadAnchorRepo
	sessionRepo *repository.LoreSessionRepo
}

// NewLoreThreadAnchorStoreAdapter creates a new LoreThreadAnchorStoreAdapter.
func NewLoreThreadAnchorStoreAdapter(anchorRepo *repository.LoreThreadAnchorRepo, sessionRepo *repository.LoreSessionRepo) *LoreThreadAnchorStoreAdapter {
	return &LoreThreadAnchorStoreAdapter{
		anchorRepo:  anchorRepo,
		sessionRepo: sessionRepo,
	}
}

func (a *LoreThreadAnchorStoreAdapter) Create(ctx context.Context, sessionID, threadID, messageID int64) error {
	session, err := a.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("session %d not found for anchor creation", sessionID)
	}

	sourceSessionID := sessionID
	summaryMsgID := messageID
	anchor := &domain.LoreThreadAnchor{
		GuildID:          session.GuildID,
		ChannelID:        session.ChannelID,
		ThreadID:         threadID,
		SummaryMessageID: &summaryMsgID,
		SourceSessionID:  &sourceSessionID,
	}

	return a.anchorRepo.Insert(ctx, anchor)
}

func (a *LoreThreadAnchorStoreAdapter) GetBySessionID(ctx context.Context, sessionID int64) (threadID, messageID int64, err error) {
	anchor, err := a.anchorRepo.GetBySession(ctx, sessionID)
	if err != nil {
		return 0, 0, err
	}
	if anchor == nil {
		return 0, 0, nil
	}

	var summaryMessageID int64
	if anchor.SummaryMessageID != nil {
		summaryMessageID = *anchor.SummaryMessageID
	}

	return anchor.ThreadID, summaryMessageID, nil
}

func (a *LoreThreadAnchorStoreAdapter) GetByThreadID(ctx context.Context, threadID int64) (sessionID int64, err error) {
	anchor, err := a.anchorRepo.GetByThreadID(ctx, threadID)
	if err != nil {
		return 0, err
	}
	if anchor == nil || anchor.SourceSessionID == nil {
		return 0, nil
	}

	return *anchor.SourceSessionID, nil
}

// CreateAnchor persists rich anchor metadata when called by lorethread.Worker.
func (a *LoreThreadAnchorStoreAdapter) CreateAnchor(ctx context.Context, anchor *lorethread.ThreadAnchor) error {
	if anchor == nil {
		return fmt.Errorf("anchor is nil")
	}

	sourceSessionID := anchor.SessionID
	summaryMsgID := anchor.MessageID
	title := anchor.Title
	summary := anchor.Summary
	dbAnchor := &domain.LoreThreadAnchor{
		GuildID:          anchor.GuildID,
		ChannelID:        anchor.ChannelID,
		ThreadID:         anchor.ThreadID,
		SummaryMessageID: &summaryMsgID,
		SummaryText:      &summary,
		Title:            &title,
		SourceSessionID:  &sourceSessionID,
	}

	return a.anchorRepo.Insert(ctx, dbAnchor)
}
