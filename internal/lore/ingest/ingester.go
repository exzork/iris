package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type LoreStore interface {
	InsertChunk(ctx context.Context, chunk LoreChunkRecord) error
	ChunkExists(ctx context.Context, pageID int64, chunkIndex int) (bool, error)
}

type LoreChunkRecord struct {
	PageID    int64
	ChunkIdx  int
	Content   string
	Embedding []float32
	URL       string
	Title     string
}

type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type Config struct {
	Client    MediaWikiClient
	Chunker   *Chunker
	Cursor    CursorStore
	Dedupe    Deduper
	Embedder  EmbeddingProvider
	Store     LoreStore
	SourceID  string
	BatchSize int
}

type Ingester struct {
	cfg Config
}

type RunStats struct {
	PagesFetched   int
	ChunksInserted int
	Skipped        int
	Errors         int
	LastID         int64
}

func New(cfg Config) *Ingester {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.Chunker == nil {
		cfg.Chunker = NewChunker(1000, 100)
	}
	return &Ingester{cfg: cfg}
}

func (i *Ingester) RunOnce(ctx context.Context) (RunStats, error) {
	var stats RunStats
	if err := i.validate(); err != nil {
		return stats, err
	}

	cur, err := i.cfg.Cursor.Get(ctx, i.cfg.SourceID)
	if err != nil {
		return stats, fmt.Errorf("ingester: load cursor: %w", err)
	}

	fromID := int64(0)
	lastSaved := &Cursor{SourceID: i.cfg.SourceID}
	if cur != nil {
		fromID = cur.LastID
		lastSaved = &Cursor{
			SourceID:  cur.SourceID,
			LastID:    cur.LastID,
			LastTitle: cur.LastTitle,
			UpdatedAt: cur.UpdatedAt,
		}
	}

	summaries, err := i.cfg.Client.ListPages(ctx, fromID, i.cfg.BatchSize)
	if err != nil {
		return stats, fmt.Errorf("ingester: list pages: %w", err)
	}

	for _, summary := range summaries {
		stats.PagesFetched++

		page, err := i.cfg.Client.GetPage(ctx, summary.ID)
		if err != nil {
			stats.Errors++
			continue
		}

		chunks := i.cfg.Chunker.Chunk(page)
		pageFailed := false
		for _, chunk := range chunks {
			exists, err := i.cfg.Store.ChunkExists(ctx, page.ID, chunk.Index)
			if err != nil {
				stats.Errors++
				pageFailed = true
				break
			}
			if exists {
				stats.Skipped++
				continue
			}

			hash := ContentHash(chunk.Content)
			seen, err := i.cfg.Dedupe.SeenHash(ctx, hash)
			if err != nil {
				stats.Errors++
				pageFailed = true
				break
			}
			if seen {
				stats.Skipped++
				continue
			}

			embedding, err := i.cfg.Embedder.Embed(ctx, chunk.Content)
			if err != nil {
				stats.Errors++
				pageFailed = true
				break
			}

			record := LoreChunkRecord{
				PageID:    page.ID,
				ChunkIdx:  chunk.Index,
				Content:   chunk.Content,
				Embedding: embedding,
				URL:       chunk.PageURL,
				Title:     chunk.Title,
			}
			if err := i.cfg.Store.InsertChunk(ctx, record); err != nil {
				stats.Errors++
				pageFailed = true
				break
			}
			if err := i.cfg.Dedupe.MarkHash(ctx, hash); err != nil {
				stats.Errors++
				pageFailed = true
				break
			}
			stats.ChunksInserted++
		}

		if pageFailed {
			continue
		}

		lastSaved.LastID = page.ID
		lastSaved.LastTitle = page.Title
		lastSaved.UpdatedAt = time.Now().UTC()
		stats.LastID = page.ID
	}

	if stats.LastID == 0 {
		stats.LastID = lastSaved.LastID
	}

	if err := i.cfg.Cursor.Save(ctx, lastSaved); err != nil {
		return stats, fmt.Errorf("ingester: save cursor: %w", err)
	}

	return stats, nil
}

func (i *Ingester) validate() error {
	if i == nil {
		return errors.New("ingester: nil ingester")
	}
	if i.cfg.Client == nil {
		return errors.New("ingester: client is required")
	}
	if i.cfg.Chunker == nil {
		return errors.New("ingester: chunker is required")
	}
	if i.cfg.Cursor == nil {
		return errors.New("ingester: cursor store is required")
	}
	if i.cfg.Dedupe == nil {
		return errors.New("ingester: deduper is required")
	}
	if i.cfg.Embedder == nil {
		return errors.New("ingester: embedder is required")
	}
	if i.cfg.Store == nil {
		return errors.New("ingester: lore store is required")
	}
	if i.cfg.SourceID == "" {
		return errors.New("ingester: source id is required")
	}
	if i.cfg.BatchSize <= 0 {
		return errors.New("ingester: batch size must be > 0")
	}
	return nil
}
