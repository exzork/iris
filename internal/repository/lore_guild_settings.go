package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/jackc/pgx/v5"
)

type LoreGuildSettingsRepo struct {
	db *DB
}

func NewLoreGuildSettingsRepo(db *DB) *LoreGuildSettingsRepo {
	return &LoreGuildSettingsRepo{db: db}
}

func (r *LoreGuildSettingsRepo) IsEnabled(ctx context.Context, guildID int64) (bool, error) {
	sql := `SELECT enabled FROM lore_guild_settings WHERE guild_id = $1`
	var enabled bool
	err := r.db.QueryRow(ctx, sql, guildID).Scan(&enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if lore is enabled: %w", err)
	}
	return enabled, nil
}

func (r *LoreGuildSettingsRepo) SetEnabled(ctx context.Context, guildID int64, enabled bool) error {
	sql := `
		INSERT INTO lore_guild_settings (guild_id, enabled, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (guild_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
	`
	_, err := r.db.Exec(ctx, sql, guildID, enabled)
	if err != nil {
		return fmt.Errorf("failed to set lore enabled status: %w", err)
	}
	return nil
}

func (r *LoreGuildSettingsRepo) GetSettings(ctx context.Context, guildID int64) (*domain.LoreGuildSettings, error) {
	sql := `SELECT guild_id, enabled, thread_cap_per_hour, created_at, updated_at FROM lore_guild_settings WHERE guild_id = $1`
	var settings domain.LoreGuildSettings
	err := r.db.QueryRow(ctx, sql, guildID).Scan(&settings.GuildID, &settings.Enabled, &settings.ThreadCapPerHour, &settings.CreatedAt, &settings.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get lore guild settings: %w", err)
	}
	return &settings, nil
}

func (r *LoreGuildSettingsRepo) IncrementThreadCount(ctx context.Context, guildID int64, hourBucket time.Time) error {
	sql := `
		INSERT INTO lore_thread_counters (guild_id, hour_bucket, count)
		VALUES ($1, $2, 1)
		ON CONFLICT (guild_id, hour_bucket) DO UPDATE SET
			count = lore_thread_counters.count + 1
	`
	_, err := r.db.Exec(ctx, sql, guildID, hourBucket)
	if err != nil {
		return fmt.Errorf("failed to increment thread count: %w", err)
	}
	return nil
}

func (r *LoreGuildSettingsRepo) CountThreadsThisHour(ctx context.Context, guildID int64, now time.Time) (int, error) {
	hourBucket := now.Truncate(time.Hour)
	sql := `SELECT COALESCE(count, 0) FROM lore_thread_counters WHERE guild_id = $1 AND hour_bucket = $2`
	var count int
	err := r.db.QueryRow(ctx, sql, guildID, hourBucket).Scan(&count)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to count threads this hour: %w", err)
	}
	return count, nil
}

func (r *LoreGuildSettingsRepo) SetThreadCapPerHour(ctx context.Context, guildID int64, cap int) error {
	sql := `
		INSERT INTO lore_guild_settings (guild_id, thread_cap_per_hour, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (guild_id) DO UPDATE SET
			thread_cap_per_hour = EXCLUDED.thread_cap_per_hour,
			updated_at = NOW()
	`
	_, err := r.db.Exec(ctx, sql, guildID, cap)
	if err != nil {
		return fmt.Errorf("failed to set thread cap per hour: %w", err)
	}
	return nil
}
