package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/pgvector/pgvector-go"
)

// ErrMissingGuildID signals that a guild-scoped query was invoked without
// a guild identifier. It is returned before any SQL executes so that
// guild isolation cannot be bypassed by a zero/empty guild argument.
var ErrMissingGuildID = errors.New("repository: guild_id is required")

// ErrInvalidVectorDim signals that a caller passed an embedding with a
// dimension that does not match the column dimension (384).
var ErrInvalidVectorDim = errors.New("repository: embedding dimension mismatch")

// ExpectedEmbeddingDim is the pgvector column dimension for
// channel_messages.content_embedding.
const ExpectedEmbeddingDim = 384

type AllowedChannelRepo struct {
	db *DB
}

func NewAllowedChannelRepo(db *DB) *AllowedChannelRepo {
	return &AllowedChannelRepo{db: db}
}

func (r *AllowedChannelRepo) Add(ctx context.Context, guildID int64, channelID int64) error {
	sql := `
		INSERT INTO allowed_channels (guild_id, channel_id, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (guild_id, channel_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, sql, guildID, channelID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add allowed channel: %w", err)
	}
	return nil
}

func (r *AllowedChannelRepo) Remove(ctx context.Context, guildID int64, channelID int64) error {
	sql := `DELETE FROM allowed_channels WHERE guild_id = $1 AND channel_id = $2`
	_, err := r.db.Exec(ctx, sql, guildID, channelID)
	if err != nil {
		return fmt.Errorf("failed to remove allowed channel: %w", err)
	}
	return nil
}

func (r *AllowedChannelRepo) IsAllowed(ctx context.Context, guildID int64, channelID int64) (bool, error) {
	sql := `SELECT EXISTS(SELECT 1 FROM allowed_channels WHERE guild_id = $1 AND channel_id = $2)`
	var exists bool
	err := r.db.QueryRow(ctx, sql, guildID, channelID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check allowed channel: %w", err)
	}
	return exists, nil
}

func (r *AllowedChannelRepo) HasAny(ctx context.Context, guildID int64) (bool, error) {
	sql := `SELECT EXISTS(SELECT 1 FROM allowed_channels WHERE guild_id = $1)`
	var exists bool
	err := r.db.QueryRow(ctx, sql, guildID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if guild has allowed channels: %w", err)
	}
	return exists, nil
}

func (r *AllowedChannelRepo) ListByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	sql := `SELECT channel_id FROM allowed_channels WHERE guild_id = $1 ORDER BY channel_id`
	rows, err := r.db.Query(ctx, sql, guildID)
	if err != nil {
		return nil, fmt.Errorf("failed to query allowed channels: %w", err)
	}
	defer rows.Close()

	var channelIDs []int64
	for rows.Next() {
		var channelID int64
		err := rows.Scan(&channelID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan allowed channel: %w", err)
		}
		channelIDs = append(channelIDs, channelID)
	}
	return channelIDs, nil
}

func (r *AllowedChannelRepo) List(ctx context.Context, guildID int64) ([]int64, error) {
	return r.ListByGuild(ctx, guildID)
}

type ChannelMessageRepo struct {
	db *DB
}

func NewChannelMessageRepo(db *DB) *ChannelMessageRepo {
	return &ChannelMessageRepo{db: db}
}

func (r *ChannelMessageRepo) Upsert(ctx context.Context, msg *domain.ChannelMessage) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var embeddingVal interface{}
	if len(msg.ContentEmbedding) > 0 {
		embeddingVal = pgvector.NewVector(msg.ContentEmbedding)
	}

	sql := `
		INSERT INTO channel_messages (
			guild_id, channel_id, message_id, user_id, author_name, content,
			attachment_count, reply_to_message_id, reply_to_channel_id, is_bot,
			trigger_source, created_at, edited_at, deleted_at, content_embedding
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (guild_id, message_id) DO UPDATE SET
			content = EXCLUDED.content,
			edited_at = EXCLUDED.edited_at,
			deleted_at = EXCLUDED.deleted_at,
			content_embedding = EXCLUDED.content_embedding
	`

	_, err = tx.Exec(ctx, sql,
		msg.GuildID, msg.ChannelID, msg.MessageID, msg.UserID, msg.AuthorName,
		msg.Content, msg.AttachmentCount, msg.ReplyToMessageID, msg.ReplyToChannelID,
		msg.IsBot, msg.TriggerSource, msg.CreatedAt, msg.EditedAt, msg.DeletedAt,
		embeddingVal,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert message: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (r *ChannelMessageRepo) PruneOldest(ctx context.Context, guildID int64, channelID int64, keep int) error {
	sql := `
		DELETE FROM channel_messages
		WHERE (guild_id, channel_id, id) IN (
			SELECT guild_id, channel_id, id
			FROM channel_messages
			WHERE guild_id = $1 AND channel_id = $2
			ORDER BY created_at DESC
			OFFSET $3
		)
	`
	_, err := r.db.Exec(ctx, sql, guildID, channelID, keep)
	if err != nil {
		return fmt.Errorf("failed to prune messages: %w", err)
	}
	return nil
}

func (r *ChannelMessageRepo) ListRecent(ctx context.Context, guildID int64, channelID int64, limit int) ([]*domain.ChannelMessage, error) {
	sql := `
		SELECT id, guild_id, channel_id, message_id, user_id, author_name, content,
		       attachment_count, reply_to_message_id, reply_to_channel_id, is_bot,
		       trigger_source, created_at, edited_at, deleted_at, content_embedding
		FROM channel_messages
		WHERE guild_id = $1 AND channel_id = $2
		ORDER BY created_at ASC
		LIMIT $3
	`
	rows, err := r.db.Query(ctx, sql, guildID, channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent messages: %w", err)
	}
	defer rows.Close()

	var messages []*domain.ChannelMessage
	for rows.Next() {
		msg := &domain.ChannelMessage{}
		var embedding *pgvector.Vector
		err := rows.Scan(
			&msg.ID, &msg.GuildID, &msg.ChannelID, &msg.MessageID, &msg.UserID,
			&msg.AuthorName, &msg.Content, &msg.AttachmentCount, &msg.ReplyToMessageID,
			&msg.ReplyToChannelID, &msg.IsBot, &msg.TriggerSource, &msg.CreatedAt,
			&msg.EditedAt, &msg.DeletedAt, &embedding,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		if embedding != nil {
			msg.ContentEmbedding = embedding.Slice()
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (r *ChannelMessageRepo) GetByID(ctx context.Context, guildID int64, messageID int64) (*domain.ChannelMessage, error) {
	sql := `
		SELECT id, guild_id, channel_id, message_id, user_id, author_name, content,
		       attachment_count, reply_to_message_id, reply_to_channel_id, is_bot,
		       trigger_source, created_at, edited_at, deleted_at, content_embedding
		FROM channel_messages
		WHERE guild_id = $1 AND message_id = $2
	`
	msg := &domain.ChannelMessage{}
	var embedding *pgvector.Vector
	err := r.db.QueryRow(ctx, sql, guildID, messageID).Scan(
		&msg.ID, &msg.GuildID, &msg.ChannelID, &msg.MessageID, &msg.UserID,
		&msg.AuthorName, &msg.Content, &msg.AttachmentCount, &msg.ReplyToMessageID,
		&msg.ReplyToChannelID, &msg.IsBot, &msg.TriggerSource, &msg.CreatedAt,
		&msg.EditedAt, &msg.DeletedAt, &embedding,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}
	if embedding != nil {
		msg.ContentEmbedding = embedding.Slice()
	}
	return msg, nil
}

func (r *ChannelMessageRepo) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	sql := `
		SELECT id, guild_id, channel_id, message_id, user_id, author_name, content,
		       attachment_count, reply_to_message_id, reply_to_channel_id, is_bot,
		       trigger_source, created_at, edited_at, deleted_at, content_embedding
		FROM channel_messages
		WHERE guild_id = $1 AND user_id = $2 AND created_at > NOW() - INTERVAL '1 minute' * $3
		ORDER BY created_at DESC
		LIMIT $4
	`
	rows, err := r.db.Query(ctx, sql, guildID, userID, sinceMinutes, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query user messages: %w", err)
	}
	defer rows.Close()

	var messages []*domain.ChannelMessage
	for rows.Next() {
		msg := &domain.ChannelMessage{}
		var embedding *pgvector.Vector
		err := rows.Scan(
			&msg.ID, &msg.GuildID, &msg.ChannelID, &msg.MessageID, &msg.UserID,
			&msg.AuthorName, &msg.Content, &msg.AttachmentCount, &msg.ReplyToMessageID,
			&msg.ReplyToChannelID, &msg.IsBot, &msg.TriggerSource, &msg.CreatedAt,
			&msg.EditedAt, &msg.DeletedAt, &embedding,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		if embedding != nil {
			msg.ContentEmbedding = embedding.Slice()
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (r *ChannelMessageRepo) MarkDeleted(ctx context.Context, guildID int64, messageID int64) error {
	sql := `UPDATE channel_messages SET deleted_at = $1 WHERE guild_id = $2 AND message_id = $3`
	_, err := r.db.Exec(ctx, sql, time.Now(), guildID, messageID)
	if err != nil {
		return fmt.Errorf("failed to mark message as deleted: %w", err)
	}
	return nil
}

func (r *ChannelMessageRepo) UpdateContent(ctx context.Context, guildID int64, messageID int64, newContent string, editedAt time.Time) error {
	sql := `UPDATE channel_messages SET content = $1, edited_at = $2 WHERE guild_id = $3 AND message_id = $4`
	_, err := r.db.Exec(ctx, sql, newContent, editedAt, guildID, messageID)
	if err != nil {
		return fmt.Errorf("failed to update message content: %w", err)
	}
	return nil
}

// PendingEmbedding is a minimal projection of a channel_messages row that is
// awaiting embedding by the async worker. The projection intentionally omits
// columns the worker does not need so the fetch is cheap.
type PendingEmbedding struct {
	ID        int64
	GuildID   int64
	MessageID int64
	Content   string
}

// ListPendingEmbeddings returns rows where content_embedding IS NULL, bounded
// by limit, across all guilds. It is used by the async embedding worker so
// the first argument is the batch cap, not a guild ID. Callers are still
// isolated per-row because updates are keyed by (guild_id, message_id).
func (r *ChannelMessageRepo) ListPendingEmbeddings(ctx context.Context, limit int) ([]*PendingEmbedding, error) {
	if limit <= 0 {
		return nil, nil
	}
	const sql = `
		SELECT id, guild_id, message_id, content
		FROM channel_messages
		WHERE content_embedding IS NULL
		  AND deleted_at IS NULL
		  AND content <> ''
		ORDER BY created_at ASC
		LIMIT $1
	`
	rows, err := r.db.Query(ctx, sql, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending embeddings: %w", err)
	}
	defer rows.Close()

	var out []*PendingEmbedding
	for rows.Next() {
		p := &PendingEmbedding{}
		if err := rows.Scan(&p.ID, &p.GuildID, &p.MessageID, &p.Content); err != nil {
			return nil, fmt.Errorf("failed to scan pending embedding: %w", err)
		}
		out = append(out, p)
	}
	return out, nil
}

// StoreEmbedding persists a 384-dim embedding for an already-captured
// channel message identified by (guild_id, message_id). Returns
// ErrMissingGuildID, ErrInvalidVectorDim, or pgx errors.
func (r *ChannelMessageRepo) StoreEmbedding(ctx context.Context, guildID int64, messageID int64, embedding []float32) error {
	if guildID == 0 {
		return ErrMissingGuildID
	}
	if len(embedding) != ExpectedEmbeddingDim {
		return fmt.Errorf("%w: got %d want %d", ErrInvalidVectorDim, len(embedding), ExpectedEmbeddingDim)
	}
	const sql = `
		UPDATE channel_messages
		SET content_embedding = $1
		WHERE guild_id = $2 AND message_id = $3
	`
	_, err := r.db.Exec(ctx, sql, pgvector.NewVector(embedding), guildID, messageID)
	if err != nil {
		return fmt.Errorf("failed to store embedding: %w", err)
	}
	return nil
}

// RecallResult is a same-guild vector recall hit plus its cosine similarity.
// Similarity is in [0,1] where 1 means identical direction.
type RecallResult struct {
	Message    *domain.ChannelMessage
	Similarity float64
}

// RecallByVector returns up to topK messages from the given guild whose
// content_embedding has cosine similarity >= threshold against the query
// vector. guildID must be non-zero; threshold must be in [0,1]; topK must
// be positive. Results are ordered by similarity descending.
func (r *ChannelMessageRepo) RecallByVector(
	ctx context.Context,
	guildID int64,
	query []float32,
	threshold float64,
	topK int,
) ([]*RecallResult, error) {
	if guildID == 0 {
		return nil, ErrMissingGuildID
	}
	if len(query) != ExpectedEmbeddingDim {
		return nil, fmt.Errorf("%w: got %d want %d", ErrInvalidVectorDim, len(query), ExpectedEmbeddingDim)
	}
	if topK <= 0 {
		return nil, nil
	}
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}

	const sql = `
		SELECT id, guild_id, channel_id, message_id, user_id, author_name, content,
		       attachment_count, reply_to_message_id, reply_to_channel_id, is_bot,
		       trigger_source, created_at, edited_at, deleted_at, content_embedding,
		       1 - (content_embedding <=> $2) AS similarity
		FROM channel_messages
		WHERE guild_id = $1
		  AND content_embedding IS NOT NULL
		  AND deleted_at IS NULL
		  AND 1 - (content_embedding <=> $2) >= $3
		ORDER BY content_embedding <=> $2 ASC
		LIMIT $4
	`
	rows, err := r.db.Query(ctx, sql, guildID, pgvector.NewVector(query), threshold, topK)
	if err != nil {
		return nil, fmt.Errorf("failed to query recall: %w", err)
	}
	defer rows.Close()

	var out []*RecallResult
	for rows.Next() {
		msg := &domain.ChannelMessage{}
		var embedding *pgvector.Vector
		var sim float64
		if err := rows.Scan(
			&msg.ID, &msg.GuildID, &msg.ChannelID, &msg.MessageID, &msg.UserID,
			&msg.AuthorName, &msg.Content, &msg.AttachmentCount, &msg.ReplyToMessageID,
			&msg.ReplyToChannelID, &msg.IsBot, &msg.TriggerSource, &msg.CreatedAt,
			&msg.EditedAt, &msg.DeletedAt, &embedding, &sim,
		); err != nil {
			return nil, fmt.Errorf("failed to scan recall row: %w", err)
		}
		if embedding != nil {
			msg.ContentEmbedding = embedding.Slice()
		}
		out = append(out, &RecallResult{Message: msg, Similarity: sim})
	}
	return out, nil
}
