package wire

import (
	"context"
	"errors"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/lorethread"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/jackc/pgx/v5"
)

// LoreSessionStoreAdapter adapts the repository.LoreSessionRepo to the lorethread.SessionStore interface.
type LoreSessionStoreAdapter struct {
	repo *repository.LoreSessionRepo
}

// NewLoreSessionStoreAdapter creates a new LoreSessionStoreAdapter.
func NewLoreSessionStoreAdapter(repo *repository.LoreSessionRepo) *LoreSessionStoreAdapter {
	return &LoreSessionStoreAdapter{repo: repo}
}

func (a *LoreSessionStoreAdapter) Create(ctx context.Context, session *lorethread.Session) error {
	if session.FirstMessage == nil {
		return nil
	}

	idleDeadline := session.UpdatedAt.Add(5 * time.Minute)
	sessionID, err := a.repo.OpenOrRefresh(
		ctx,
		session.GuildID,
		session.ChannelID,
		session.FirstMessage.ID,
		session.FirstMessage.CreatedAt,
		idleDeadline,
	)
	if err != nil {
		return err
	}

	session.ID = sessionID
	return nil
}

func (a *LoreSessionStoreAdapter) GetByID(ctx context.Context, id int64) (*lorethread.Session, error) {
	domainSession, err := a.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if domainSession == nil {
		return nil, nil
	}

	return a.domainToLorethread(domainSession), nil
}

func (a *LoreSessionStoreAdapter) GetActive(ctx context.Context, guildID, channelID int64) (*lorethread.Session, error) {
	domainSession, err := a.repo.GetOpenByChannel(ctx, guildID, channelID)
	if err != nil {
		return nil, err
	}

	if domainSession == nil {
		return nil, nil
	}

	return a.domainToLorethread(domainSession), nil
}

func (a *LoreSessionStoreAdapter) Update(ctx context.Context, session *lorethread.Session) error {
	if session.FirstMessage == nil || len(session.Messages) == 0 {
		return nil
	}

	lastMsg := session.Messages[len(session.Messages)-1]
	idleDeadline := session.UpdatedAt.Add(5 * time.Minute)

	_, err := a.repo.OpenOrRefresh(
		ctx,
		session.GuildID,
		session.ChannelID,
		lastMsg.ID,
		lastMsg.CreatedAt,
		idleDeadline,
	)
	return err
}

func (a *LoreSessionStoreAdapter) ListByGuild(ctx context.Context, guildID int64) ([]*lorethread.Session, error) {
	return nil, nil
}

// ClaimDueForSummary is the worker-oriented extension used by lorethread.Worker.
func (a *LoreSessionStoreAdapter) ClaimDueForSummary(ctx context.Context, now time.Time) (*lorethread.Session, error) {
	domainSession, err := a.repo.ClaimDueForSummary(ctx, now)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, lorethread.ErrNoSessionsDue
		}
		return nil, err
	}

	if domainSession == nil {
		return nil, lorethread.ErrNoSessionsDue
	}

	return a.domainToLorethread(domainSession), nil
}

// MarkStatus is the worker-oriented extension used by lorethread.Worker.
func (a *LoreSessionStoreAdapter) MarkStatus(ctx context.Context, id int64, status string) error {
	return a.repo.MarkStatus(ctx, id, status)
}

// SetThreadResult is the worker-oriented extension used by lorethread.Worker.
func (a *LoreSessionStoreAdapter) SetThreadResult(ctx context.Context, id int64, threadID int64, summaryMsgID int64, title string, summary string) error {
	return a.repo.SetThreadResult(ctx, id, threadID, summaryMsgID, title, summary)
}

// IncrementRetry is the worker-oriented extension used by lorethread.Worker.
func (a *LoreSessionStoreAdapter) IncrementRetry(ctx context.Context, id int64, lastErr string) error {
	return a.repo.IncrementRetry(ctx, id, lastErr)
}

// domainToLorethread converts a domain.LoreSession to a lorethread.Session.
func (a *LoreSessionStoreAdapter) domainToLorethread(ds *domain.LoreSession) *lorethread.Session {
	session := &lorethread.Session{
		ID:                 ds.ID,
		GuildID:            ds.GuildID,
		ChannelID:          ds.ChannelID,
		FirstLoreMessageID: ds.FirstLoreMessageID,
		LastLoreMessageID:  ds.LastLoreMessageID,
		LastLoreMessageAt:  ds.LastLoreMessageAt,
		IdleDeadline:       ds.IdleDeadline,
		Status:             ds.Status,
		RetryCount:         ds.RetryCount,
		LastError:          ds.LastError,
		ThreadID:           ds.ThreadID,
		SummaryMessageID:   ds.SummaryMessageID,
		Title:              ds.Title,
		Summary:            ds.Summary,
		CreatedAt:          ds.CreatedAt,
		UpdatedAt:          ds.UpdatedAt,
		IsActive:           ds.Status == "open",
	}

	if ds.FirstLoreMessageID > 0 {
		session.FirstMessage = &lorethread.Message{
			ID:        ds.FirstLoreMessageID,
			GuildID:   ds.GuildID,
			ChannelID: ds.ChannelID,
		}
	}

	return session
}
