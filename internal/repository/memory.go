package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/pgvector/pgvector-go"
)

type MemoryRepo struct {
	db *DB
}

func NewMemoryRepo(db *DB) *MemoryRepo {
	return &MemoryRepo{db: db}
}

func (r *MemoryRepo) Save(ctx context.Context, guildID int64, userID int64, content string, embedding []float32) error {
	sql := `
		INSERT INTO memory_records (guild_id, user_id, content, embedding, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	vec := pgvector.NewVector(embedding)
	_, err := r.db.Exec(ctx, sql, guildID, nullableUserID(userID), content, vec, time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to save memory record: %w", err)
	}
	return nil
}

func nullableUserID(userID int64) interface{} {
	if userID == 0 {
		return nil
	}
	return userID
}

func (r *MemoryRepo) GetByGuild(ctx context.Context, guildID int64, limit int) ([]domain.MemoryRecord, error) {
	sql := `
		SELECT id, guild_id, content, embedding, created_at, updated_at
		FROM memory_records
		WHERE guild_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.Query(ctx, sql, guildID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query memory records: %w", err)
	}
	defer rows.Close()

	var records []domain.MemoryRecord
	for rows.Next() {
		var record domain.MemoryRecord
		var vec pgvector.Vector
		err := rows.Scan(&record.ID, &record.GuildID, &record.Content, &vec, &record.CreatedAt, &record.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan memory record: %w", err)
		}
		record.Embedding = vec.Slice()
		records = append(records, record)
	}
	return records, nil
}

func (r *MemoryRepo) SearchSimilar(ctx context.Context, guildID int64, embedding []float32, limit int) ([]domain.MemoryRecord, error) {
	sql := `
		SELECT id, guild_id, content, embedding, created_at, updated_at
		FROM memory_records
		WHERE guild_id = $1
		ORDER BY embedding <-> $2
		LIMIT $3
	`
	vec := pgvector.NewVector(embedding)
	rows, err := r.db.Query(ctx, sql, guildID, vec, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar memories: %w", err)
	}
	defer rows.Close()

	var records []domain.MemoryRecord
	for rows.Next() {
		var record domain.MemoryRecord
		var vec pgvector.Vector
		err := rows.Scan(&record.ID, &record.GuildID, &record.Content, &vec, &record.CreatedAt, &record.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan memory record: %w", err)
		}
		record.Embedding = vec.Slice()
		records = append(records, record)
	}
	return records, nil
}

func (r *MemoryRepo) Delete(ctx context.Context, guildID int64, recordID int64) error {
	sql := `DELETE FROM memory_records WHERE guild_id = $1 AND id = $2`
	_, err := r.db.Exec(ctx, sql, guildID, recordID)
	if err != nil {
		return fmt.Errorf("failed to delete memory record: %w", err)
	}
	return nil
}
