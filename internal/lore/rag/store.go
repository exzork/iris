package rag

import "context"

// ChunkStore defines the interface for retrieving chunks by similarity.
type ChunkStore interface {
	SearchSimilar(ctx context.Context, embedding []float32, topK int) ([]ScoredChunk, error)
}

// InMemoryChunkStore is a test helper that stores chunks in memory.
type InMemoryChunkStore struct {
	Chunks []Chunk
}

// SearchSimilar returns the top-K chunks by cosine similarity to the given embedding.
func (s *InMemoryChunkStore) SearchSimilar(ctx context.Context, embedding []float32, topK int) ([]ScoredChunk, error) {
	if len(embedding) == 0 || len(s.Chunks) == 0 {
		return nil, nil
	}

	scored := make([]ScoredChunk, 0, len(s.Chunks))
	for _, chunk := range s.Chunks {
		if len(chunk.Embedding) != len(embedding) {
			continue
		}
		score := cosineSimilarity(embedding, chunk.Embedding)
		scored = append(scored, ScoredChunk{Chunk: chunk, Score: score})
	}

	// Sort descending by score
	for i := range scored {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].Score > scored[i].Score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	if topK < len(scored) {
		scored = scored[:topK]
	}

	return scored, nil
}

// cosineSimilarity computes cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (sqrt(na) * sqrt(nb))
}

// sqrt computes square root using Newton's method for stdlib-only implementation.
func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x == 0 {
		return 0
	}
	z := x
	for range 10 {
		z = (z + x/z) / 2
	}
	return z
}
