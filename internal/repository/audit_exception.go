package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type AuditRepo struct {
	db *DB
}

func NewAuditRepo(db *DB) *AuditRepo {
	return &AuditRepo{db: db}
}

func (r *AuditRepo) Log(ctx context.Context, guildID int64, userID int64, eventType, entityType, entityID string, changes map[string]interface{}) error {
	changesJSON, _ := json.Marshal(changes)

	sql := `
		INSERT INTO audit_events (guild_id, user_id, event_type, entity_type, entity_id, changes, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.Exec(ctx, sql, guildID, userID, eventType, entityType, entityID, changesJSON, time.Now())
	if err != nil {
		return fmt.Errorf("failed to log audit event: %w", err)
	}
	return nil
}

func (r *AuditRepo) GetByGuild(ctx context.Context, guildID int64, limit int) ([]map[string]interface{}, error) {
	sql := `
		SELECT id, guild_id, user_id, event_type, entity_type, entity_id, changes, created_at
		FROM audit_events
		WHERE guild_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.Query(ctx, sql, guildID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var id int64
		var gid int64
		var uid int64
		var eventType, entityType, entityID string
		var changesJSON []byte
		var createdAt time.Time

		err := rows.Scan(&id, &gid, &uid, &eventType, &entityType, &entityID, &changesJSON, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit event: %w", err)
		}

		event := map[string]interface{}{
			"id":          id,
			"guild_id":    gid,
			"user_id":     uid,
			"event_type":  eventType,
			"entity_type": entityType,
			"entity_id":   entityID,
			"created_at":  createdAt,
		}

		var changes map[string]interface{}
		json.Unmarshal(changesJSON, &changes)
		event["changes"] = changes

		events = append(events, event)
	}
	return events, nil
}

type ExceptionChannelRepo struct {
	db *DB
}

func NewExceptionChannelRepo(db *DB) *ExceptionChannelRepo {
	return &ExceptionChannelRepo{db: db}
}

func (r *ExceptionChannelRepo) Add(ctx context.Context, guildID int64, channelID int64) error {
	sql := `
		INSERT INTO exception_channels (guild_id, channel_id, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (guild_id, channel_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, sql, guildID, channelID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add exception channel: %w", err)
	}
	return nil
}

func (r *ExceptionChannelRepo) IsException(ctx context.Context, guildID int64, channelID int64) (bool, error) {
	sql := `SELECT EXISTS(SELECT 1 FROM exception_channels WHERE guild_id = $1 AND channel_id = $2)`
	var exists bool
	err := r.db.QueryRow(ctx, sql, guildID, channelID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check exception channel: %w", err)
	}
	return exists, nil
}

func (r *ExceptionChannelRepo) GetByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	sql := `SELECT channel_id FROM exception_channels WHERE guild_id = $1 ORDER BY channel_id`
	rows, err := r.db.Query(ctx, sql, guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to query exception channels: %w", err)
	}
	defer rows.Close()

	var channelIDs []int64
	for rows.Next() {
		var channelID int64
		err := rows.Scan(&channelID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan exception channel: %w", err)
		}
		channelIDs = append(channelIDs, channelID)
	}
	return channelIDs, nil
}

func (r *ExceptionChannelRepo) Remove(ctx context.Context, guildID int64, channelID int64) error {
	sql := `DELETE FROM exception_channels WHERE guild_id = $1 AND channel_id = $2`
	_, err := r.db.Exec(ctx, sql, guildID, channelID)
	if err != nil {
		return fmt.Errorf("failed to remove exception channel: %w", err)
	}
	return nil
}

func (r *ExceptionChannelRepo) List(ctx context.Context, guildID int64) ([]int64, error) {
	return r.GetByGuild(ctx, guildID)
}
