package websearch

import (
	"context"
)

// SearchResult represents a single search result from a provider.
type SearchResult struct {
	Title          string
	URL            string
	Snippet        string
	Source         string // provider name e.g. "duckduckgo", "brave"
	Authoritative  bool   // true if URL is canon-authoritative for WuWa
}

// Provider defines the interface for search providers.
type Provider interface {
	// Search performs a search query and returns results.
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
	// Name returns the provider's name.
	Name() string
}
