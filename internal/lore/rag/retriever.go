package rag

import "context"

type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type Retriever struct {
	Embed    EmbeddingProvider
	Store    ChunkStore
	MinScore float64
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

	return filtered, nil
}
