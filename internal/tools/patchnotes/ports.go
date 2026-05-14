package patchnotes

import (
	"context"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
	websearchpkg "github.com/eko/iris-bot/internal/tools/websearch"
)

// SearchPort defines the interface for web search functionality.
type SearchPort interface {
	Search(ctx context.Context, query string, limit int) ([]websearchpkg.SearchResult, error)
}

// RAGPort defines the interface for retrieval-augmented generation functionality.
type RAGPort interface {
	Retrieve(ctx context.Context, query string, topK int) ([]ragpkg.ScoredChunk, error)
}
