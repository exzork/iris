package ingest

import (
	"strings"
	"testing"
)

func TestChunkSplitsLongText(t *testing.T) {
	chunker := NewChunker(120, 0)
	text := strings.Repeat("Paragraph one sentence. ", 8) + "\n\n" + strings.Repeat("Paragraph two sentence. ", 8)

	chunks := chunker.Chunk(&Page{ID: 1, Title: "T", URL: "https://wiki/test", Wikitext: text})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for _, ch := range chunks {
		if len(ch.Content) > 120 {
			t.Fatalf("chunk exceeded max chars (%d): %d", 120, len(ch.Content))
		}
	}
}

func TestChunkOverlap(t *testing.T) {
	chunker := NewChunker(50, 10)
	text := strings.Repeat("Alpha sentence. ", 10)
	chunks := chunker.Chunk(&Page{ID: 2, Title: "Overlap", URL: "https://wiki/overlap", Wikitext: text})
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	prev := chunks[0].Content
	next := chunks[1].Content
	if len(prev) < 10 || len(next) < 10 {
		t.Fatalf("chunks too small for overlap check")
	}
	if prev[len(prev)-10:] != next[:10] {
		t.Fatalf("expected overlap to match tail/head; tail=%q head=%q", prev[len(prev)-10:], next[:10])
	}
}

func TestChunkShortFits(t *testing.T) {
	chunker := NewChunker(200, 20)
	chunks := chunker.Chunk(&Page{ID: 3, Title: "Short", URL: "https://wiki/short", Wikitext: "tiny content"})
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "tiny content" {
		t.Fatalf("unexpected content: %q", chunks[0].Content)
	}
}

func TestChunkMetadataPreserved(t *testing.T) {
	page := &Page{ID: 99, Title: "Meta", URL: "https://wiki/meta", Wikitext: strings.Repeat("meta sentence. ", 10)}
	chunks := NewChunker(80, 8).Chunk(page)
	if len(chunks) == 0 {
		t.Fatalf("expected chunks")
	}
	for i, ch := range chunks {
		if ch.PageID != page.ID {
			t.Fatalf("chunk %d missing page id", i)
		}
		if ch.PageURL != page.URL {
			t.Fatalf("chunk %d missing page url", i)
		}
		if ch.Title != page.Title {
			t.Fatalf("chunk %d missing title", i)
		}
		if ch.Index != i {
			t.Fatalf("expected index %d got %d", i, ch.Index)
		}
	}
}
