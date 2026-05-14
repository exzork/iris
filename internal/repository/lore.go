package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/pgvector/pgvector-go"
)

type LoreRepo struct {
	db *DB
}

func NewLoreRepo(db *DB) *LoreRepo {
	return &LoreRepo{db: db}
}

func (r *LoreRepo) CreateDocument(ctx context.Context, guildID int64, title, content string) (int64, error) {
	sql := `
		INSERT INTO lore_documents (guild_id, title, content, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`
	var docID int64
	err := r.db.QueryRow(ctx, sql, guildID, title, content, time.Now(), time.Now()).Scan(&docID)
	if err != nil {
		return 0, fmt.Errorf("failed to create lore document: %w", err)
	}
	return docID, nil
}

func (r *LoreRepo) SaveChunk(ctx context.Context, guildID int64, documentID int64, chunkText string, embedding []float32, chunkIndex int) error {
	sql := `
		INSERT INTO lore_chunks (guild_id, document_id, chunk_text, embedding, chunk_index, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	vec := pgvector.NewVector(embedding)
	_, err := r.db.Exec(ctx, sql, guildID, documentID, chunkText, vec, chunkIndex, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save lore chunk: %w", err)
	}
	return nil
}

func (r *LoreRepo) SearchChunks(ctx context.Context, guildID int64, embedding []float32, limit int) ([]domain.LoreCitation, error) {
	sql := `
		SELECT lc.id, lc.guild_id, ld.title, lc.chunk_text, '', lc.created_at
		FROM lore_chunks lc
		JOIN lore_documents ld ON lc.document_id = ld.id
		WHERE lc.guild_id = $1
		ORDER BY lc.embedding <-> $2
		LIMIT $3
	`
	vec := pgvector.NewVector(embedding)
	rows, err := r.db.Query(ctx, sql, guildID, vec, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search lore chunks: %w", err)
	}
	defer rows.Close()

	var citations []domain.LoreCitation
	for rows.Next() {
		var citation domain.LoreCitation
		err := rows.Scan(&citation.ID, &citation.GuildID, &citation.Source, &citation.Content, &citation.URL, &citation.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lore citation: %w", err)
		}
		citations = append(citations, citation)
	}
	return citations, nil
}

func (r *LoreRepo) GetChunksByDocument(ctx context.Context, guildID int64, documentID int64) ([]domain.LoreCitation, error) {
	sql := `
		SELECT lc.id, lc.guild_id, ld.title, lc.chunk_text, '', lc.created_at
		FROM lore_chunks lc
		JOIN lore_documents ld ON lc.document_id = ld.id
		WHERE lc.guild_id = $1 AND lc.document_id = $2
		ORDER BY lc.chunk_index
	`
	rows, err := r.db.Query(ctx, sql, guildID, documentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query lore chunks: %w", err)
	}
	defer rows.Close()

	var citations []domain.LoreCitation
	for rows.Next() {
		var citation domain.LoreCitation
		err := rows.Scan(&citation.ID, &citation.GuildID, &citation.Source, &citation.Content, &citation.URL, &citation.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lore citation: %w", err)
		}
		citations = append(citations, citation)
	}
	return citations, nil
}

func (r *LoreRepo) DeleteDocument(ctx context.Context, guildID int64, documentID int64) error {
	sql := `DELETE FROM lore_documents WHERE guild_id = $1 AND id = $2`
	_, err := r.db.Exec(ctx, sql, guildID, documentID)
	if err != nil {
		return fmt.Errorf("failed to delete lore document: %w", err)
	}
	return nil
}
