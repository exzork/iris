package wire

import (
	"context"

	"github.com/eko/iris-bot/internal/lore/ingest"
	"github.com/eko/iris-bot/internal/lore/rag"
	"github.com/eko/iris-bot/internal/repository"
)

type WikiCursorAdapter struct {
	Repo     *repository.WikiRepo
	SourceID string
}

func (a *WikiCursorAdapter) Get(ctx context.Context, sourceID string) (*ingest.Cursor, error) {
	rec, err := a.Repo.GetCursor(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return &ingest.Cursor{
		SourceID:  rec.SourceID,
		LastID:    rec.LastPageID,
		LastTitle: rec.LastTitle,
		UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (a *WikiCursorAdapter) Save(ctx context.Context, cur *ingest.Cursor) error {
	if cur == nil {
		return nil
	}
	return a.Repo.SaveCursor(ctx, &repository.WikiCursor{
		SourceID:   cur.SourceID,
		LastTitle:  cur.LastTitle,
		LastPageID: cur.LastID,
		UpdatedAt:  cur.UpdatedAt,
	})
}

type WikiDedupeAdapter struct {
	Repo     *repository.WikiRepo
	SourceID string
}

func (a *WikiDedupeAdapter) SeenHash(ctx context.Context, hash string) (bool, error) {
	return a.Repo.HashSeen(ctx, a.SourceID, hash)
}

func (a *WikiDedupeAdapter) MarkHash(ctx context.Context, hash string) error {
	return a.Repo.MarkHash(ctx, a.SourceID, hash)
}

type WikiIngestStoreAdapter struct {
	Repo     *repository.WikiRepo
	SourceID string
}

func (a *WikiIngestStoreAdapter) ChunkExists(ctx context.Context, pageID int64, chunkIndex int) (bool, error) {
	return a.Repo.ChunkExists(ctx, a.SourceID, pageID, chunkIndex)
}

func (a *WikiIngestStoreAdapter) InsertChunk(ctx context.Context, chunk ingest.LoreChunkRecord) error {
	if err := a.Repo.UpsertPage(ctx, a.SourceID, chunk.PageID, chunk.Title, chunk.URL, 0); err != nil {
		return err
	}
	return a.Repo.InsertChunk(
		ctx,
		a.SourceID,
		chunk.PageID,
		chunk.ChunkIdx,
		chunk.Content,
		ingest.ContentHash(chunk.Content),
		chunk.Title,
		chunk.URL,
		chunk.Embedding,
	)
}

type WikiRetrievalAdapter struct {
	Repo     *repository.WikiRepo
	SourceID string
}

func (a *WikiRetrievalAdapter) SearchSimilar(ctx context.Context, embedding []float32, topK int) ([]rag.ScoredChunk, error) {
	results, err := a.Repo.SearchSimilar(ctx, a.SourceID, embedding, topK)
	if err != nil {
		return nil, err
	}
	out := make([]rag.ScoredChunk, 0, len(results))
	for _, r := range results {
		out = append(out, rag.ScoredChunk{
			Chunk: rag.Chunk{
				ID:      r.ID,
				PageID:  r.PageID,
				Title:   r.Title,
				URL:     r.URL,
				Content: r.Content,
			},
			Score: 1 - r.Distance,
		})
	}
	return out, nil
}
