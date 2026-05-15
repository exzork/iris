package wire

import (
	"context"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
)

// CuratedMemoryAdapter implements orchestrator.CuratedMemorySource backed by
// memory_records. Embeds the query with the local ONNX embedder (matching
// the column dimension), pulls top-K hits via cosine, and applies a minimum
// similarity threshold so noise doesn't pollute the prompt.
type CuratedMemoryAdapter struct {
	Repo      *repository.MemoryRepo
	Embedder  embedder.Embedder
	MinScore  float64
}

func (a *CuratedMemoryAdapter) Recall(ctx context.Context, guildID int64, query string, topK int) ([]domain.MemoryRecord, error) {
	if a.Repo == nil || a.Embedder == nil || guildID == 0 || query == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}
	threshold := a.MinScore
	if threshold <= 0 {
		threshold = 0.40
	}

	vec, err := a.Embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	rows, err := a.Repo.SearchSimilar(ctx, guildID, vec, topK)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MemoryRecord, 0, len(rows))
	for _, r := range rows {
		if r.Similarity < threshold {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
