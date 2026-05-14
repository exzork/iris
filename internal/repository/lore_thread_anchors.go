package repository

import (
	"context"
	"fmt"

	"github.com/eko/iris-bot/internal/domain"
)

type LoreThreadAnchorRepo struct {
	db *DB
}

func NewLoreThreadAnchorRepo(db *DB) *LoreThreadAnchorRepo {
	return &LoreThreadAnchorRepo{db: db}
}

func (r *LoreThreadAnchorRepo) Insert(ctx context.Context, anchor *domain.LoreThreadAnchor) error {
	sql := `
		INSERT INTO lore_thread_anchors (guild_id, channel_id, thread_id, summary_message_id, summary_text, title, source_session_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`
	_, err := r.db.Exec(ctx, sql, anchor.GuildID, anchor.ChannelID, anchor.ThreadID, anchor.SummaryMessageID, anchor.SummaryText, anchor.Title, anchor.SourceSessionID)
	if err != nil {
		return fmt.Errorf("failed to insert lore thread anchor: %w", err)
	}
	return nil
}

func (r *LoreThreadAnchorRepo) GetByThread(ctx context.Context, guildID int64, threadID int64) (*domain.LoreThreadAnchor, error) {
	sql := `
		SELECT id, guild_id, channel_id, thread_id, summary_message_id, summary_text, title, source_session_id, created_at
		FROM lore_thread_anchors
		WHERE guild_id = $1 AND thread_id = $2
	`
	var anchor domain.LoreThreadAnchor
	err := r.db.QueryRow(ctx, sql, guildID, threadID).Scan(
		&anchor.ID, &anchor.GuildID, &anchor.ChannelID, &anchor.ThreadID, &anchor.SummaryMessageID, &anchor.SummaryText, &anchor.Title, &anchor.SourceSessionID, &anchor.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get lore thread anchor: %w", err)
	}
	return &anchor, nil
}

func (r *LoreThreadAnchorRepo) GetBySession(ctx context.Context, sessionID int64) (*domain.LoreThreadAnchor, error) {
	sql := `
		SELECT id, guild_id, channel_id, thread_id, summary_message_id, summary_text, title, source_session_id, created_at
		FROM lore_thread_anchors
		WHERE source_session_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var anchor domain.LoreThreadAnchor
	err := r.db.QueryRow(ctx, sql, sessionID).Scan(
		&anchor.ID, &anchor.GuildID, &anchor.ChannelID, &anchor.ThreadID, &anchor.SummaryMessageID, &anchor.SummaryText, &anchor.Title, &anchor.SourceSessionID, &anchor.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get lore thread anchor by session: %w", err)
	}
	return &anchor, nil
}

func (r *LoreThreadAnchorRepo) GetByThreadID(ctx context.Context, threadID int64) (*domain.LoreThreadAnchor, error) {
	sql := `
		SELECT id, guild_id, channel_id, thread_id, summary_message_id, summary_text, title, source_session_id, created_at
		FROM lore_thread_anchors
		WHERE thread_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var anchor domain.LoreThreadAnchor
	err := r.db.QueryRow(ctx, sql, threadID).Scan(
		&anchor.ID, &anchor.GuildID, &anchor.ChannelID, &anchor.ThreadID, &anchor.SummaryMessageID, &anchor.SummaryText, &anchor.Title, &anchor.SourceSessionID, &anchor.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get lore thread anchor by thread id: %w", err)
	}
	return &anchor, nil
}
