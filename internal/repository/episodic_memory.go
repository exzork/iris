package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/pgvector/pgvector-go"
)

type EpisodeMemoryRepo struct {
	db *DB
}

func NewEpisodeMemoryRepo(db *DB) *EpisodeMemoryRepo {
	return &EpisodeMemoryRepo{db: db}
}

func (r *EpisodeMemoryRepo) Save(ctx context.Context, ep *domain.EpisodeMemory) error {
	if ep == nil {
		return fmt.Errorf("episode is nil")
	}
	if ep.GuildID == 0 {
		return ErrMissingGuildID
	}
	if len(ep.Embedding) != 0 && len(ep.Embedding) != ExpectedEmbeddingDim {
		return ErrInvalidVectorDim
	}

	occurredAt := ep.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}

	var vec *pgvector.Vector
	if len(ep.Embedding) == ExpectedEmbeddingDim {
		v := pgvector.NewVector(ep.Embedding)
		vec = &v
	}

	sql := `
		INSERT INTO episodic_memory (
			guild_id, channel_id, thread_id, channel_name, thread_name,
			user_id, author_name, message_id, content, tagged_line,
			embedding, embedding_model, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (guild_id, message_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, sql,
		ep.GuildID, ep.ChannelID, ep.ThreadID, ep.ChannelName, ep.ThreadName,
		ep.UserID, ep.AuthorName, ep.MessageID, ep.Content, ep.TaggedLine,
		vec, ep.EmbeddingModel, occurredAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save episode: %w", err)
	}
	return nil
}

func (r *EpisodeMemoryRepo) SaveBatch(ctx context.Context, episodes []*domain.EpisodeMemory) (int, error) {
	if len(episodes) == 0 {
		return 0, nil
	}
	saved := 0
	for _, ep := range episodes {
		if err := r.Save(ctx, ep); err != nil {
			return saved, err
		}
		saved++
	}
	return saved, nil
}

type EpisodeRecallResult struct {
	Episode    *domain.EpisodeMemory
	Similarity float32
}

func (r *EpisodeMemoryRepo) SearchSimilar(ctx context.Context, guildID int64, embedding []float32, limit int, threshold float32) ([]*EpisodeRecallResult, error) {
	if guildID == 0 {
		return nil, ErrMissingGuildID
	}
	if len(embedding) != ExpectedEmbeddingDim {
		return nil, ErrInvalidVectorDim
	}
	if limit <= 0 {
		limit = 5
	}

	vec := pgvector.NewVector(embedding)
	sql := `
		SELECT id, guild_id, channel_id, thread_id, channel_name, thread_name,
		       user_id, author_name, message_id, content, tagged_line,
		       embedding_model, occurred_at, archived_at, deleted_at,
		       1 - (embedding <=> $2) AS similarity
		FROM episodic_memory
		WHERE guild_id = $1
		  AND embedding IS NOT NULL
		  AND deleted_at IS NULL
		  AND 1 - (embedding <=> $2) >= $3
		ORDER BY embedding <=> $2
		LIMIT $4
	`
	rows, err := r.db.Query(ctx, sql, guildID, vec, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search episodes: %w", err)
	}
	defer rows.Close()

	var out []*EpisodeRecallResult
	for rows.Next() {
		ep := &domain.EpisodeMemory{}
		var sim float32
		if err := rows.Scan(
			&ep.ID, &ep.GuildID, &ep.ChannelID, &ep.ThreadID, &ep.ChannelName, &ep.ThreadName,
			&ep.UserID, &ep.AuthorName, &ep.MessageID, &ep.Content, &ep.TaggedLine,
			&ep.EmbeddingModel, &ep.OccurredAt, &ep.ArchivedAt, &ep.DeletedAt, &sim,
		); err != nil {
			return nil, fmt.Errorf("failed to scan episode: %w", err)
		}
		out = append(out, &EpisodeRecallResult{Episode: ep, Similarity: sim})
	}
	return out, nil
}

type PendingEpisodeEmbedding struct {
	ID      int64
	GuildID int64
	Content string
}

func (r *EpisodeMemoryRepo) ListPendingEmbeddings(ctx context.Context, limit int) ([]*PendingEpisodeEmbedding, error) {
	if limit <= 0 {
		limit = 100
	}
	sql := `
		SELECT id, guild_id, content
		FROM episodic_memory
		WHERE embedding IS NULL AND deleted_at IS NULL
		ORDER BY id ASC
		LIMIT $1
	`
	rows, err := r.db.Query(ctx, sql, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending episode embeddings: %w", err)
	}
	defer rows.Close()

	var out []*PendingEpisodeEmbedding
	for rows.Next() {
		p := &PendingEpisodeEmbedding{}
		if err := rows.Scan(&p.ID, &p.GuildID, &p.Content); err != nil {
			return nil, fmt.Errorf("failed to scan pending episode: %w", err)
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *EpisodeMemoryRepo) UpdateEmbedding(ctx context.Context, id int64, embedding []float32, model string) error {
	if len(embedding) != ExpectedEmbeddingDim {
		return ErrInvalidVectorDim
	}
	vec := pgvector.NewVector(embedding)
	sql := `
		UPDATE episodic_memory
		SET embedding = $1, embedding_model = $2
		WHERE id = $3
	`
	_, err := r.db.Exec(ctx, sql, vec, model, id)
	if err != nil {
		return fmt.Errorf("failed to update episode embedding: %w", err)
	}
	return nil
}
