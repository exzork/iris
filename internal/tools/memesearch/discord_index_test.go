package memesearch

import (
	"context"
	"testing"
)

func TestInMemoryDiscordIndexFindsGIF(t *testing.T) {
	idx := NewInMemoryDiscordIndex()
	idx.AddMessage(12345, "reaksi kaget", "https://media.tenor.com/example.gif", "image/gif")

	results, err := idx.Search(context.Background(), 12345, "reaksi kaget", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	item := results[0]
	if item.URL != "https://media.tenor.com/example.gif" {
		t.Errorf("expected URL https://media.tenor.com/example.gif, got %s", item.URL)
	}
	if item.Source != SourceDiscordHistory {
		t.Errorf("expected source discord_history, got %s", item.Source)
	}
	if item.MimeType != "image/gif" {
		t.Errorf("expected mime type image/gif, got %s", item.MimeType)
	}
	if item.Caption != "reaksi kaget" {
		t.Errorf("expected caption 'reaksi kaget', got %s", item.Caption)
	}
}
