package repository

import (
	"context"
	"fmt"
	"time"
)

type ChannelConversationRepo struct {
	db *DB
}

func NewChannelConversationRepo(db *DB) *ChannelConversationRepo {
	return &ChannelConversationRepo{db: db}
}

func (r *ChannelConversationRepo) Refresh(ctx context.Context, guildID int64, channelID int64, now time.Time, ttl time.Duration) error {
	lockUntil := now.Add(ttl)
	sql := `
		INSERT INTO channel_conversations (guild_id, channel_id, last_bot_reply_at, lock_until, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (guild_id, channel_id) DO UPDATE SET
			last_bot_reply_at = EXCLUDED.last_bot_reply_at,
			lock_until = EXCLUDED.lock_until,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.Exec(ctx, sql, guildID, channelID, now, lockUntil, now)
	if err != nil {
		return fmt.Errorf("failed to refresh channel conversation: %w", err)
	}
	return nil
}

func (r *ChannelConversationRepo) Active(ctx context.Context, guildID int64, channelID int64, now time.Time) (bool, error) {
	sql := `
		SELECT EXISTS(
			SELECT 1 FROM channel_conversations
			WHERE guild_id = $1 AND channel_id = $2 AND lock_until > $3
		)
	`
	var exists bool
	err := r.db.QueryRow(ctx, sql, guildID, channelID, now).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check active conversation: %w", err)
	}
	return exists, nil
}

func (r *ChannelConversationRepo) Clear(ctx context.Context, guildID int64, channelID int64) error {
	sql := `DELETE FROM channel_conversations WHERE guild_id = $1 AND channel_id = $2`
	_, err := r.db.Exec(ctx, sql, guildID, channelID)
	if err != nil {
		return fmt.Errorf("failed to clear channel conversation: %w", err)
	}
	return nil
}
