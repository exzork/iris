package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

type GuildRepo struct {
	db *DB
}

func NewGuildRepo(db *DB) *GuildRepo {
	return &GuildRepo{db: db}
}

func (r *GuildRepo) Create(ctx context.Context, guild *domain.Guild) error {
	sql := `INSERT INTO guilds (id, name, created_at, updated_at) VALUES ($1, $2, $3, $4)`
	_, err := r.db.Exec(ctx, sql, guild.ID, "Guild", time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to create guild: %w", err)
	}
	return nil
}

func (r *GuildRepo) GetByID(ctx context.Context, guildID int64) (*domain.Guild, error) {
	sql := `SELECT id, created_at, updated_at FROM guilds WHERE id = $1`
	row := r.db.QueryRow(ctx, sql, guildID)

	var guild domain.Guild
	err := row.Scan(&guild.ID, &guild.CreatedAt, &guild.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get guild: %w", err)
	}
	return &guild, nil
}

func (r *GuildRepo) Delete(ctx context.Context, guildID int64) error {
	sql := `DELETE FROM guilds WHERE id = $1`
	_, err := r.db.Exec(ctx, sql, guildID)
	if err != nil {
		return fmt.Errorf("failed to delete guild: %w", err)
	}
	return nil
}

func (r *GuildRepo) EnsureGuild(ctx context.Context, guildID int64) error {
	sql := `INSERT INTO guilds (id, name, created_at, updated_at) VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO NOTHING`
	_, err := r.db.Exec(ctx, sql, guildID, "Guild", time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to ensure guild: %w", err)
	}
	return nil
}
