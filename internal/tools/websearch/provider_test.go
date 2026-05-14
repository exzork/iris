package websearch

import (
	"context"
	"testing"
)

type FakeProvider struct {
	results []SearchResult
	err     error
}

func NewFakeProvider(results []SearchResult, err error) *FakeProvider {
	return &FakeProvider{results: results, err: err}
}

func (f *FakeProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if limit < len(f.results) {
		return f.results[:limit], nil
	}
	return f.results, nil
}

func (f *FakeProvider) Name() string {
	return "fake"
}

func TestFakeProviderReturnsResults(t *testing.T) {
	results := []SearchResult{
		{
			Title:   "Test Result 1",
			URL:     "https://example.com/1",
			Snippet: "This is a test result",
			Source:  "fake",
		},
		{
			Title:   "Test Result 2",
			URL:     "https://example.com/2",
			Snippet: "Another test result",
			Source:  "fake",
		},
	}

	provider := NewFakeProvider(results, nil)
	got, err := provider.Search(context.Background(), "test", 10)

	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if len(got) != len(results) {
		t.Fatalf("Search() returned %d results, want %d", len(got), len(results))
	}

	for i, result := range got {
		if result.Title != results[i].Title {
			t.Errorf("result[%d].Title = %q, want %q", i, result.Title, results[i].Title)
		}
		if result.URL != results[i].URL {
			t.Errorf("result[%d].URL = %q, want %q", i, result.URL, results[i].URL)
		}
	}
}
