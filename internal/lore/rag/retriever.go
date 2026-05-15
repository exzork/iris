package rag

import (
	"context"
	"log/slog"
)

type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type Retriever struct {
	Embed    EmbeddingProvider
	Store    ChunkStore
	MinScore float64
	Logger   *slog.Logger
}

func (r *Retriever) Retrieve(ctx context.Context, query string, topK int) ([]ScoredChunk, error) {
	if query == "" {
		return nil, nil
	}

	embedding, err := r.Embed.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	scored, err := r.Store.SearchSimilar(ctx, embedding, topK)
	if err != nil {
		return nil, err
	}

	filtered := make([]ScoredChunk, 0, len(scored))
	for _, chunk := range scored {
		if chunk.Score >= r.MinScore {
			filtered = append(filtered, chunk)
		}
	}

	r.logRetrieval(ctx, query, topK, filtered)
	return filtered, nil
}

func (r *Retriever) logRetrieval(ctx context.Context, query string, topK int, hits []ScoredChunk) {
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}

	titles := make([]string, len(hits))
	urls := make([]string, len(hits))
	scores := make([]float64, len(hits))
	for i, hit := range hits {
		titles[i] = hit.Title
		urls[i] = hit.URL
		scores[i] = hit.Score
	}

	logger.InfoContext(ctx, "wiki_retrieval",
		"query", truncateForLog(query, 200),
		"top_k", topK,
		"hits", len(hits),
		"min_score", r.MinScore,
		"titles", titles,
		"urls", urls,
		"scores", scores,
	)
}

func truncateForLog(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
