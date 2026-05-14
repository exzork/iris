package memesearch

import (
	"context"
	"testing"
)

type FakeSocialAdapter struct {
	results []MediaItem
	src     Source
}

func NewFakeSocialAdapter(src Source, results []MediaItem) *FakeSocialAdapter {
	return &FakeSocialAdapter{
		results: results,
		src:     src,
	}
}

func (f *FakeSocialAdapter) Search(ctx context.Context, query string, limit int) ([]MediaItem, error) {
	if limit > len(f.results) {
		return f.results, nil
	}
	return f.results[:limit], nil
}

func (f *FakeSocialAdapter) Source() Source {
	return f.src
}

func TestFakeAdapterReturnsResults(t *testing.T) {
	results := []MediaItem{
		{
			URL:      "https://example.com/meme1.gif",
			Source:   SourceX,
			MimeType: "image/gif",
			Caption:  "funny meme",
			Safety:   SafetySafe,
		},
	}

	adapter := NewFakeSocialAdapter(SourceX, results)

	got, err := adapter.Search(context.Background(), "funny", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}

	if got[0].URL != "https://example.com/meme1.gif" {
		t.Errorf("expected URL https://example.com/meme1.gif, got %s", got[0].URL)
	}

	if adapter.Source() != SourceX {
		t.Errorf("expected source x, got %s", adapter.Source())
	}
}
