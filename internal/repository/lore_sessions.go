package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

type LoreSessionRepo struct {
	db *DB
}

func NewLoreSessionRepo(db *DB) *LoreSessionRepo {
	return &LoreSessionRepo{db: db}
}

func (r *LoreSessionRepo) OpenOrRefresh(ctx context.Context, guildID int64, channelID int64, msgID int64, msgTime time.Time, idleDeadline time.Time) (int64, error) {
	sql := `
		INSERT INTO lore_sessions (guild_id, channel_id, first_lore_message_id, last_lore_message_id, last_lore_message_at, idle_deadline, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'open', NOW(), NOW())
		ON CONFLICT (guild_id, channel_id) WHERE status = 'open' DO UPDATE SET
			last_lore_message_id = EXCLUDED.last_lore_message_id,
			last_lore_message_at = EXCLUDED.last_lore_message_at,
			idle_deadline = EXCLUDED.idle_deadline,
			updated_at = NOW()
		RETURNING id
	`
	var sessionID int64
	err := r.db.QueryRow(ctx, sql, guildID, channelID, msgID, msgID, msgTime, idleDeadline).Scan(&sessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to open or refresh lore session: %w", err)
	}
	return sessionID, nil
}

func (r *LoreSessionRepo) GetOpenByChannel(ctx context.Context, guildID int64, channelID int64) (*domain.LoreSession, error) {
	sql := `
		SELECT id, guild_id, channel_id, first_lore_message_id, last_lore_message_id, last_lore_message_at, idle_deadline, status, title, summary, thread_id, summary_message_id, retry_count, last_error, created_at, updated_at
		FROM lore_sessions
		WHERE guild_id = $1 AND channel_id = $2 AND status = 'open'
	`
	var session domain.LoreSession
	err := r.db.QueryRow(ctx, sql, guildID, channelID).Scan(
		&session.ID, &session.GuildID, &session.ChannelID, &session.FirstLoreMessageID, &session.LastLoreMessageID, &session.LastLoreMessageAt, &session.IdleDeadline, &session.Status, &session.Title, &session.Summary, &session.ThreadID, &session.SummaryMessageID, &session.RetryCount, &session.LastError, &session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get open lore session: %w", err)
	}
	return &session, nil
}

// GetOpenByChannelWithStarter retrieves the open lore session for a channel and its starter user ID.
// Returns the session, starter user ID, and error. If no open session exists, returns sql.ErrNoRows.
// If the first message cannot be found (rare race), returns the session with starterID=0.
func (r *LoreSessionRepo) GetOpenByChannelWithStarter(ctx context.Context, guildID int64, channelID int64) (*domain.LoreSession, int64, error) {
	sql := `
		SELECT ls.id, ls.guild_id, ls.channel_id, ls.first_lore_message_id, ls.last_lore_message_id, ls.last_lore_message_at, ls.idle_deadline, ls.status, ls.title, ls.summary, ls.thread_id, ls.summary_message_id, ls.retry_count, ls.last_error, ls.created_at, ls.updated_at, cm.user_id
		FROM lore_sessions ls
		LEFT JOIN channel_messages cm ON cm.id = ls.first_lore_message_id
		WHERE ls.guild_id = $1 AND ls.channel_id = $2 AND ls.status = 'open'
	`
	var session domain.LoreSession
	var starterID int64
	err := r.db.QueryRow(ctx, sql, guildID, channelID).Scan(
		&session.ID, &session.GuildID, &session.ChannelID, &session.FirstLoreMessageID, &session.LastLoreMessageID, &session.LastLoreMessageAt, &session.IdleDeadline, &session.Status, &session.Title, &session.Summary, &session.ThreadID, &session.SummaryMessageID, &session.RetryCount, &session.LastError, &session.CreatedAt, &session.UpdatedAt, &starterID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get open lore session with starter: %w", err)
	}
	return &session, starterID, nil
}

func (r *LoreSessionRepo) GetByID(ctx context.Context, id int64) (*domain.LoreSession, error) {
	sql := `
		SELECT id, guild_id, channel_id, first_lore_message_id, last_lore_message_id, last_lore_message_at, idle_deadline, status, title, summary, thread_id, summary_message_id, retry_count, last_error, created_at, updated_at
		FROM lore_sessions
		WHERE id = $1
	`
	var session domain.LoreSession
	err := r.db.QueryRow(ctx, sql, id).Scan(
		&session.ID, &session.GuildID, &session.ChannelID, &session.FirstLoreMessageID, &session.LastLoreMessageID, &session.LastLoreMessageAt, &session.IdleDeadline, &session.Status, &session.Title, &session.Summary, &session.ThreadID, &session.SummaryMessageID, &session.RetryCount, &session.LastError, &session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get lore session by id: %w", err)
	}
	return &session, nil
}

func (r *LoreSessionRepo) ClaimDueForSummary(ctx context.Context, now time.Time) (*domain.LoreSession, error) {
	sql := `
		SELECT id, guild_id, channel_id, first_lore_message_id, last_lore_message_id, last_lore_message_at, idle_deadline, status, title, summary, thread_id, summary_message_id, retry_count, last_error, created_at, updated_at
		FROM lore_sessions
		WHERE status IN ('open', 'summarizing') AND idle_deadline <= $1
		ORDER BY idle_deadline ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`
	var session domain.LoreSession
	err := r.db.QueryRow(ctx, sql, now).Scan(
		&session.ID, &session.GuildID, &session.ChannelID, &session.FirstLoreMessageID, &session.LastLoreMessageID, &session.LastLoreMessageAt, &session.IdleDeadline, &session.Status, &session.Title, &session.Summary, &session.ThreadID, &session.SummaryMessageID, &session.RetryCount, &session.LastError, &session.CreatedAt, &session.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to claim lore session due for summary: %w", err)
	}
	return &session, nil
}

func (r *LoreSessionRepo) MarkStatus(ctx context.Context, id int64, status string) error {
	sql := `UPDATE lore_sessions SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, sql, status, id)
	if err != nil {
		return fmt.Errorf("failed to mark lore session status: %w", err)
	}
	return nil
}

func (r *LoreSessionRepo) SetThreadResult(ctx context.Context, id int64, threadID int64, summaryMsgID int64, title string, summary string) error {
	sql := `
		UPDATE lore_sessions
		SET thread_id = $1, summary_message_id = $2, title = $3, summary = $4, status = 'thread_created', updated_at = NOW()
		WHERE id = $5
	`
	_, err := r.db.Exec(ctx, sql, threadID, summaryMsgID, title, summary, id)
	if err != nil {
		return fmt.Errorf("failed to set lore session thread result: %w", err)
	}
	return nil
}

func (r *LoreSessionRepo) DeleteAllByGuild(ctx context.Context, guildID int64) (int64, error) {
	sql := `DELETE FROM lore_sessions WHERE guild_id = $1`
	tag, err := r.db.Exec(ctx, sql, guildID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete lore sessions by guild: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *LoreSessionRepo) IncrementRetry(ctx context.Context, id int64, lastErr string) error {
	sql := `
		UPDATE lore_sessions
		SET retry_count = retry_count + 1, last_error = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.db.Exec(ctx, sql, lastErr, id)
	if err != nil {
		return fmt.Errorf("failed to increment lore session retry: %w", err)
	}
	return nil
}
