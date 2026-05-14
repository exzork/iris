package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

type SettingsRepo struct {
	db *DB
}

func NewSettingsRepo(db *DB) *SettingsRepo {
	return &SettingsRepo{db: db}
}

func (r *SettingsRepo) Save(ctx context.Context, config *domain.GuildConfig) error {
	sql := `
		INSERT INTO guild_settings (guild_id, setting_key, setting_value, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (guild_id, setting_key) DO UPDATE
		SET setting_value = $3, updated_at = $5
	`
	_, err := r.db.Exec(ctx, sql, config.GuildID, config.SettingKey, config.SettingValue, time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to save guild config: %w", err)
	}
	return nil
}

func (r *SettingsRepo) GetByKey(ctx context.Context, guildID int64, key string) (*domain.GuildConfig, error) {
	sql := `SELECT id, guild_id, setting_key, setting_value, created_at, updated_at FROM guild_settings WHERE guild_id = $1 AND setting_key = $2`
	row := r.db.QueryRow(ctx, sql, guildID, key)

	var config domain.GuildConfig
	err := row.Scan(&config.ID, &config.GuildID, &config.SettingKey, &config.SettingValue, &config.CreatedAt, &config.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get guild config: %w", err)
	}
	return &config, nil
}

func (r *SettingsRepo) GetAllByGuild(ctx context.Context, guildID int64) ([]domain.GuildConfig, error) {
	sql := `SELECT id, guild_id, setting_key, setting_value, created_at, updated_at FROM guild_settings WHERE guild_id = $1 ORDER BY setting_key`
	rows, err := r.db.Query(ctx, sql, guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to query guild configs: %w", err)
	}
	defer rows.Close()

	var configs []domain.GuildConfig
	for rows.Next() {
		var config domain.GuildConfig
		err := rows.Scan(&config.ID, &config.GuildID, &config.SettingKey, &config.SettingValue, &config.CreatedAt, &config.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan guild config: %w", err)
		}
		configs = append(configs, config)
	}
	return configs, nil
}

func (r *SettingsRepo) Delete(ctx context.Context, guildID int64, key string) error {
	sql := `DELETE FROM guild_settings WHERE guild_id = $1 AND setting_key = $2`
	_, err := r.db.Exec(ctx, sql, guildID, key)
	if err != nil {
		return fmt.Errorf("failed to delete guild config: %w", err)
	}
	return nil
}
