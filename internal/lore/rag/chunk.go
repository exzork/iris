package rag

// Chunk represents a single indexed piece of lore content.
type Chunk struct {
	ID        int64
	PageID    int64
	Title     string
	URL       string
	Content   string
	Embedding []float32
}

// ScoredChunk wraps a Chunk with its retrieval score.
type ScoredChunk struct {
	Chunk
	Score float64 // cosine similarity; higher = better
}
