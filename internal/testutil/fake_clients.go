package testutil

import (
	"context"
	"time"
)

// FakeEmbeddingClient implements domain.EmbeddingClient for testing.
type FakeEmbeddingClient struct {
	EmbedFn         func(ctx context.Context, text string) ([]float32, error)
	SimulateLatency time.Duration
	SimulateError   error
	EmbedDim        int
	CallLog         []string
}

// NewFakeEmbeddingClient creates a new fake embedding client.
func NewFakeEmbeddingClient() *FakeEmbeddingClient {
	return &FakeEmbeddingClient{
		EmbedDim: 1536,
		CallLog:  []string{},
	}
}

// Embed generates an embedding vector.
func (f *FakeEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if f.SimulateLatency > 0 {
		select {
		case <-time.After(f.SimulateLatency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if f.SimulateError != nil {
		return nil, f.SimulateError
	}

	if f.EmbedFn != nil {
		return f.EmbedFn(ctx, text)
	}

	f.CallLog = append(f.CallLog, text)

	vec := make([]float32, f.EmbedDim)
	for i := range vec {
		vec[i] = float32(len(text)) / float32(f.EmbedDim)
	}
	return vec, nil
}

// GetCallLog returns all embed calls for verification.
func (f *FakeEmbeddingClient) GetCallLog() []string {
	return append([]string{}, f.CallLog...)
}

// FakeImageClient implements domain.ImageClient for testing.
type FakeImageClient struct {
	GenerateFn      func(ctx context.Context, prompt string) (string, error)
	SimulateLatency time.Duration
	SimulateError   error
	GeneratedURLs   []string
}

// NewFakeImageClient creates a new fake image client.
func NewFakeImageClient() *FakeImageClient {
	return &FakeImageClient{
		GeneratedURLs: []string{},
	}
}

// Generate generates an image.
func (f *FakeImageClient) Generate(ctx context.Context, prompt string) (string, error) {
	if f.SimulateLatency > 0 {
		select {
		case <-time.After(f.SimulateLatency):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if f.SimulateError != nil {
		return "", f.SimulateError
	}

	if f.GenerateFn != nil {
		return f.GenerateFn(ctx, prompt)
	}

	url := "https://example.com/image.png"
	f.GeneratedURLs = append(f.GeneratedURLs, url)
	return url, nil
}

// GetGeneratedURLs returns all generated image URLs for verification.
func (f *FakeImageClient) GetGeneratedURLs() []string {
	return append([]string{}, f.GeneratedURLs...)
}

// FakeWebSearch implements a web search client for testing.
type FakeWebSearch struct {
	SearchFn        func(ctx context.Context, query string) ([]string, error)
	SimulateLatency time.Duration
	SimulateError   error
	Results         map[string][]string
	CallLog         []string
}

// NewFakeWebSearch creates a new fake web search client.
func NewFakeWebSearch() *FakeWebSearch {
	return &FakeWebSearch{
		Results: make(map[string][]string),
		CallLog: []string{},
	}
}

// Search performs a web search.
func (f *FakeWebSearch) Search(ctx context.Context, query string) ([]string, error) {
	if f.SimulateLatency > 0 {
		select {
		case <-time.After(f.SimulateLatency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if f.SimulateError != nil {
		return nil, f.SimulateError
	}

	if f.SearchFn != nil {
		return f.SearchFn(ctx, query)
	}

	f.CallLog = append(f.CallLog, query)

	if results, exists := f.Results[query]; exists {
		return results, nil
	}

	return []string{"https://example.com/result1", "https://example.com/result2"}, nil
}

// GetCallLog returns all search queries for verification.
func (f *FakeWebSearch) GetCallLog() []string {
	return append([]string{}, f.CallLog...)
}
