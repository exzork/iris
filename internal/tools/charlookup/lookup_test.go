package charlookup

import (
	"context"
	"testing"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
)

func TestLookupFindByAlias(t *testing.T) {
	store := NewInMemoryStore()
	char := &Character{
		ID:      1,
		Name:    "Rover",
		Aliases: []string{"protagonist"},
		Element: "Spectro",
		Weapon:  "Sword",
		Summary: "Rover adalah karakter utama.",
	}
	store.Add(char)

	idx := NewAliasIndex()
	idx.Add(char)

	lookup := &Lookup{
		Store: store,
		Alias: idx,
	}

	result, err := lookup.Find(context.Background(), "protagonist")
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if !result.Found {
		t.Fatal("Expected character to be found")
	}
	if result.Character.ID != 1 {
		t.Errorf("Character ID: got %d, want 1", result.Character.ID)
	}
	if result.Summary != char.Summary {
		t.Errorf("Summary: got %q, want %q", result.Summary, char.Summary)
	}
}

func TestLookupMissingReturnsIndonesianMessage(t *testing.T) {
	store := NewInMemoryStore()
	idx := NewAliasIndex()

	lookup := &Lookup{
		Store: store,
		Alias: idx,
	}

	result, err := lookup.Find(context.Background(), "TidakAda999")
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if result.Found {
		t.Fatal("Expected character not to be found")
	}
	if result.Missing == "" {
		t.Fatal("Expected Indonesian missing message")
	}
	if !contains(result.Missing, "TidakAda999") {
		t.Errorf("Missing message should contain query: %q", result.Missing)
	}
	if !contains(result.Missing, "belum terindeks") {
		t.Errorf("Missing message should contain 'belum terindeks': %q", result.Missing)
	}
}

func TestLookupUsesRetrieverForSnippets(t *testing.T) {
	store := NewInMemoryStore()
	char := &Character{
		ID:      1,
		Name:    "Rover",
		Summary: "Rover adalah karakter utama.",
	}
	store.Add(char)

	idx := NewAliasIndex()
	idx.Add(char)

	fakeRetriever := &fakeRetriever{
		chunks: []ragpkg.ScoredChunk{
			{
				Chunk: ragpkg.Chunk{
					Title:   "Rover Character Page",
					URL:     "https://wutheringwaves.fandom.com/wiki/Rover",
					Content: "Rover memiliki elemen Spectro.",
				},
				Score: 0.95,
			},
			{
				Chunk: ragpkg.Chunk{
					Title:   "Rover Skills",
					URL:     "https://wutheringwaves.fandom.com/wiki/Rover/Skills",
					Content: "Skill utama Rover adalah Echo Strike.",
				},
				Score: 0.85,
			},
		},
	}

	lookup := &Lookup{
		Store:     store,
		Alias:     idx,
		Retriever: &ragpkg.Retriever{},
	}
	lookup.Retriever.Embed = fakeRetriever
	lookup.Retriever.Store = fakeRetriever

	result, err := lookup.Find(context.Background(), "Rover")
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if !result.Found {
		t.Fatal("Expected character to be found")
	}
	if len(result.Citations) == 0 {
		t.Fatal("Expected citations from retriever")
	}
	if !contains(result.Summary, "Spectro") {
		t.Errorf("Summary should contain RAG snippet: %q", result.Summary)
	}
}

func TestLookupRetrieverNilUsesFallbackSummary(t *testing.T) {
	store := NewInMemoryStore()
	char := &Character{
		ID:      1,
		Name:    "Rover",
		Summary: "Rover adalah karakter utama.",
	}
	store.Add(char)

	idx := NewAliasIndex()
	idx.Add(char)

	lookup := &Lookup{
		Store:     store,
		Alias:     idx,
		Retriever: nil,
	}

	result, err := lookup.Find(context.Background(), "Rover")
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if !result.Found {
		t.Fatal("Expected character to be found")
	}
	if result.Summary != char.Summary {
		t.Errorf("Summary: got %q, want %q", result.Summary, char.Summary)
	}
	if len(result.Citations) != 0 {
		t.Errorf("Expected no citations without retriever: got %d", len(result.Citations))
	}
}

func TestLookupEmptyQuery(t *testing.T) {
	store := NewInMemoryStore()
	idx := NewAliasIndex()

	lookup := &Lookup{
		Store: store,
		Alias: idx,
	}

	result, err := lookup.Find(context.Background(), "")
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}

	if result.Found {
		t.Fatal("Expected character not to be found for empty query")
	}
	if result.Missing == "" {
		t.Fatal("Expected Indonesian missing message")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type fakeRetriever struct {
	chunks []ragpkg.ScoredChunk
}

func (f *fakeRetriever) Embed(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, 384), nil
}

func (f *fakeRetriever) SearchSimilar(ctx context.Context, embedding []float32, topK int) ([]ragpkg.ScoredChunk, error) {
	if len(f.chunks) > topK {
		return f.chunks[:topK], nil
	}
	return f.chunks, nil
}
