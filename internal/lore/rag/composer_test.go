package rag

import (
	"context"
	"testing"
)

func TestComposeSupportedAnswer(t *testing.T) {
	store := &InMemoryChunkStore{
		Chunks: []Chunk{
			{
				ID:        1,
				PageID:    1,
				Title:     "Rover",
				URL:       "https://wutheringwaves.fandom.com/wiki/Rover",
				Content:   "Rover is a character in Wuthering Waves.",
				Embedding: []float32{1, 0, 0, 0},
			},
		},
	}

	retriever := &Retriever{
		Embed:    &FakeEmbedder{embedding: []float32{1, 0, 0, 0}},
		Store:    store,
		MinScore: 0.0,
	}

	composer := &Composer{
		Retriever: retriever,
		MinChunks: 1,
	}

	promptCtx, unsupported, err := composer.Compose(context.Background(), "Who is Rover?")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	if promptCtx == nil {
		t.Fatalf("expected PromptContext, got nil")
	}

	if unsupported != nil {
		t.Fatalf("expected nil UnsupportedResponse, got %v", unsupported)
	}

	if !promptCtx.HasSupport {
		t.Fatalf("expected HasSupport=true")
	}

	if len(promptCtx.Citations) != 1 {
		t.Fatalf("expected 1 citation, got %d", len(promptCtx.Citations))
	}

	if promptCtx.Citations[0].Title != "Rover" {
		t.Fatalf("expected citation title 'Rover', got %q", promptCtx.Citations[0].Title)
	}

	if len(promptCtx.Snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(promptCtx.Snippets))
	}

	t.Logf("PromptContext: %+v", promptCtx)
}

func TestComposeUnsupportedCaveat(t *testing.T) {
	store := &InMemoryChunkStore{
		Chunks: []Chunk{},
	}

	retriever := &Retriever{
		Embed:    &FakeEmbedder{embedding: []float32{1, 0, 0, 0}},
		Store:    store,
		MinScore: 0.0,
	}

	composer := &Composer{
		Retriever: retriever,
		MinChunks: 1,
	}

	promptCtx, unsupported, err := composer.Compose(context.Background(), "Tell me about an unknown theory")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	if promptCtx != nil {
		t.Fatalf("expected nil PromptContext, got %v", promptCtx)
	}

	if unsupported == nil {
		t.Fatalf("expected UnsupportedResponse, got nil")
	}

	expected := "Belum ada data yang terindeks untuk pertanyaan ini. Silakan periksa wiki Wuthering Waves secara langsung: https://wutheringwaves.fandom.com/wiki/Special:Search"
	if unsupported.Message != expected {
		t.Fatalf("expected message %q, got %q", expected, unsupported.Message)
	}

	t.Logf("UnsupportedResponse: %+v", unsupported)
}

func TestComposeMultipleSourcesDedupedAndSorted(t *testing.T) {
	store := &InMemoryChunkStore{
		Chunks: []Chunk{
			{
				ID:        1,
				PageID:    1,
				Title:     "Rover",
				URL:       "https://wutheringwaves.fandom.com/wiki/Rover",
				Content:   "Rover is a character.",
				Embedding: []float32{1, 0, 0, 0},
			},
			{
				ID:        2,
				PageID:    1,
				Title:     "Rover (continued)",
				URL:       "https://wutheringwaves.fandom.com/wiki/Rover",
				Content:   "Rover has abilities.",
				Embedding: []float32{0.99, 0.01, 0, 0},
			},
			{
				ID:        3,
				PageID:    2,
				Title:     "Jinhsi",
				URL:       "https://wutheringwaves.fandom.com/wiki/Jinhsi",
				Content:   "Jinhsi is another character.",
				Embedding: []float32{0.9, 0.1, 0, 0},
			},
		},
	}

	retriever := &Retriever{
		Embed:    &FakeEmbedder{embedding: []float32{1, 0, 0, 0}},
		Store:    store,
		MinScore: 0.0,
	}

	composer := &Composer{
		Retriever: retriever,
		MinChunks: 1,
	}

	promptCtx, unsupported, err := composer.Compose(context.Background(), "Tell me about characters")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	if promptCtx == nil {
		t.Fatalf("expected PromptContext, got nil")
	}

	if unsupported != nil {
		t.Fatalf("expected nil UnsupportedResponse, got %v", unsupported)
	}

	if len(promptCtx.Citations) != 2 {
		t.Fatalf("expected 2 deduplicated citations, got %d", len(promptCtx.Citations))
	}

	if promptCtx.Citations[0].URL != "https://wutheringwaves.fandom.com/wiki/Rover" {
		t.Fatalf("expected first citation URL to be Rover, got %q", promptCtx.Citations[0].URL)
	}

	if promptCtx.Citations[1].URL != "https://wutheringwaves.fandom.com/wiki/Jinhsi" {
		t.Fatalf("expected second citation URL to be Jinhsi, got %q", promptCtx.Citations[1].URL)
	}

	if len(promptCtx.Snippets) != 3 {
		t.Fatalf("expected 3 snippets (all chunks), got %d", len(promptCtx.Snippets))
	}
}

func TestComposeEmptyQueryReturnsUnsupported(t *testing.T) {
	store := &InMemoryChunkStore{
		Chunks: []Chunk{
			{
				ID:        1,
				PageID:    1,
				Title:     "Rover",
				URL:       "https://wutheringwaves.fandom.com/wiki/Rover",
				Content:   "Rover is a character.",
				Embedding: []float32{1, 0, 0, 0},
			},
		},
	}

	retriever := &Retriever{
		Embed:    &FakeEmbedder{embedding: []float32{1, 0, 0, 0}},
		Store:    store,
		MinScore: 0.0,
	}

	composer := &Composer{
		Retriever: retriever,
		MinChunks: 1,
	}

	promptCtx, unsupported, err := composer.Compose(context.Background(), "")
	if err != nil {
		t.Fatalf("Compose failed: %v", err)
	}

	if promptCtx != nil {
		t.Fatalf("expected nil PromptContext, got %v", promptCtx)
	}

	if unsupported == nil {
		t.Fatalf("expected UnsupportedResponse, got nil")
	}

	expected := "Belum ada data yang terindeks untuk pertanyaan ini. Silakan periksa wiki Wuthering Waves secara langsung: https://wutheringwaves.fandom.com/wiki/Special:Search"
	if unsupported.Message != expected {
		t.Fatalf("expected message %q, got %q", expected, unsupported.Message)
	}
}
