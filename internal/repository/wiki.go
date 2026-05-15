package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

type WikiRepo struct {
	db *DB
}

func NewWikiRepo(db *DB) *WikiRepo {
	return &WikiRepo{db: db}
}

type WikiCursor struct {
	SourceID          string
	LastTitle         string
	LastPageID        int64
	ContinuationToken string
	UpdatedAt         time.Time
}

func (r *WikiRepo) GetCursor(ctx context.Context, sourceID string) (*WikiCursor, error) {
	const sql = `
		SELECT source_id, last_title, last_page_id, continuation_token, updated_at
		FROM wiki_ingest_cursors
		WHERE source_id = $1
	`
	var cur WikiCursor
	err := r.db.QueryRow(ctx, sql, sourceID).Scan(&cur.SourceID, &cur.LastTitle, &cur.LastPageID, &cur.ContinuationToken, &cur.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("wiki cursor get: %w", err)
	}
	return &cur, nil
}

func (r *WikiRepo) SaveCursor(ctx context.Context, cur *WikiCursor) error {
	if cur == nil {
		return errors.New("wiki cursor: nil")
	}
	const sql = `
		INSERT INTO wiki_ingest_cursors (source_id, last_title, last_page_id, continuation_token, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (source_id) DO UPDATE
		SET last_title = EXCLUDED.last_title,
			last_page_id = EXCLUDED.last_page_id,
			continuation_token = EXCLUDED.continuation_token,
			updated_at = EXCLUDED.updated_at
	`
	updated := cur.UpdatedAt
	if updated.IsZero() {
		updated = time.Now().UTC()
	}
	if _, err := r.db.Exec(ctx, sql, cur.SourceID, cur.LastTitle, cur.LastPageID, cur.ContinuationToken, updated); err != nil {
		return fmt.Errorf("wiki cursor save: %w", err)
	}
	return nil
}

func (r *WikiRepo) UpsertPage(ctx context.Context, sourceID string, pageID int64, title, url string, revision int64) error {
	const sql = `
		INSERT INTO wiki_pages (source_id, page_id, title, url, revision, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (source_id, page_id) DO UPDATE
		SET title = EXCLUDED.title,
			url = EXCLUDED.url,
			revision = EXCLUDED.revision,
			updated_at = EXCLUDED.updated_at
	`
	if _, err := r.db.Exec(ctx, sql, sourceID, pageID, sanitizeUTF8(title), sanitizeUTF8(url), revision); err != nil {
		return fmt.Errorf("wiki page upsert: %w", err)
	}
	return nil
}

func (r *WikiRepo) ChunkExists(ctx context.Context, sourceID string, pageID int64, chunkIndex int) (bool, error) {
	const sql = `
		SELECT 1 FROM wiki_chunks
		WHERE source_id = $1 AND page_id = $2 AND chunk_index = $3
		LIMIT 1
	`
	var found int
	err := r.db.QueryRow(ctx, sql, sourceID, pageID, chunkIndex).Scan(&found)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("wiki chunk exists: %w", err)
	}
	return true, nil
}

func (r *WikiRepo) InsertChunk(ctx context.Context, sourceID string, pageID int64, chunkIndex int, content, contentHash, title, url string, embedding []float32) error {
	const sql = `
		INSERT INTO wiki_chunks (source_id, page_id, chunk_index, content, content_hash, embedding, title, url, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (source_id, page_id, chunk_index) DO NOTHING
	`
	vec := pgvector.NewVector(embedding)
	if _, err := r.db.Exec(ctx, sql, sourceID, pageID, chunkIndex, sanitizeUTF8(content), contentHash, vec, sanitizeUTF8(title), sanitizeUTF8(url)); err != nil {
		return fmt.Errorf("wiki chunk insert: %w", err)
	}
	return nil
}

func (r *WikiRepo) HashSeen(ctx context.Context, sourceID, hash string) (bool, error) {
	const sql = `
		SELECT 1 FROM wiki_content_hashes
		WHERE source_id = $1 AND content_hash = $2
		LIMIT 1
	`
	var found int
	err := r.db.QueryRow(ctx, sql, sourceID, hash).Scan(&found)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("wiki hash seen: %w", err)
	}
	return true, nil
}

func (r *WikiRepo) MarkHash(ctx context.Context, sourceID, hash string) error {
	const sql = `
		INSERT INTO wiki_content_hashes (source_id, content_hash)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	if _, err := r.db.Exec(ctx, sql, sourceID, hash); err != nil {
		return fmt.Errorf("wiki hash mark: %w", err)
	}
	return nil
}

type WikiSearchResult struct {
	ID       int64
	PageID   int64
	Title    string
	URL      string
	Content  string
	Distance float64
}

func (r *WikiRepo) SearchSimilar(ctx context.Context, sourceID string, embedding []float32, topK int) ([]WikiSearchResult, error) {
	if topK <= 0 {
		topK = 5
	}
	const sql = `
		SELECT id, page_id, title, url, content, embedding <=> $2 AS distance
		FROM wiki_chunks
		WHERE source_id = $1
		ORDER BY embedding <=> $2
		LIMIT $3
	`
	vec := pgvector.NewVector(embedding)
	rows, err := r.db.Query(ctx, sql, sourceID, vec, topK)
	if err != nil {
		return nil, fmt.Errorf("wiki search: %w", err)
	}
	defer rows.Close()

	out := make([]WikiSearchResult, 0, topK)
	for rows.Next() {
		var rec WikiSearchResult
		if err := rows.Scan(&rec.ID, &rec.PageID, &rec.Title, &rec.URL, &rec.Content, &rec.Distance); err != nil {
			return nil, fmt.Errorf("wiki search scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("wiki search rows: %w", err)
	}
	return out, nil
}

func (r *WikiRepo) ChunkCount(ctx context.Context, sourceID string) (int64, error) {
	const sql = `SELECT COUNT(*) FROM wiki_chunks WHERE source_id = $1`
	var n int64
	if err := r.db.QueryRow(ctx, sql, sourceID).Scan(&n); err != nil {
		return 0, fmt.Errorf("wiki chunk count: %w", err)
	}
	return n, nil
}

func (r *WikiRepo) PageCount(ctx context.Context, sourceID string) (int64, error) {
	const sql = `SELECT COUNT(*) FROM wiki_pages WHERE source_id = $1`
	var n int64
	if err := r.db.QueryRow(ctx, sql, sourceID).Scan(&n); err != nil {
		return 0, fmt.Errorf("wiki page count: %w", err)
	}
	return n, nil
}

// sanitizeUTF8 replaces invalid UTF-8 byte sequences with U+FFFD and strips NULs.
// Wikitext occasionally contains stray Latin-1 bytes (e.g. 0xa6) that Postgres
// rejects with SQLSTATE 22021. NUL bytes are also illegal in TEXT columns.
func sanitizeUTF8(s string) string {
	if s == "" {
		return s
	}
	if utf8.ValidString(s) && !strings.ContainsRune(s, 0) {
		return s
	}
	cleaned := strings.ToValidUTF8(s, "\uFFFD")
	if strings.ContainsRune(cleaned, 0) {
		cleaned = strings.ReplaceAll(cleaned, "\x00", "")
	}
	return cleaned
}
