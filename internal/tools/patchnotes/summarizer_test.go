package patchnotes

import (
	"context"
	"testing"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
	websearchpkg "github.com/eko/iris-bot/internal/tools/websearch"
)

type fakeSearchPort struct {
	results []websearchpkg.SearchResult
}

func (f *fakeSearchPort) Search(ctx context.Context, query string, limit int) ([]websearchpkg.SearchResult, error) {
	return f.results, nil
}

type fakeRAGPort struct {
	chunks []ragpkg.ScoredChunk
}

func (f *fakeRAGPort) Retrieve(ctx context.Context, query string, topK int) ([]ragpkg.ScoredChunk, error) {
	return f.chunks, nil
}

func TestSummarizeOfficialAndWiki(t *testing.T) {
	search := &fakeSearchPort{
		results: []websearchpkg.SearchResult{
			{
				Title:   "Patch 1.4 Official Notes",
				Snippet: "New characters and balance changes",
				URL:     "https://wutheringwaves.com/patch-1.4",
			},
			{
				Title:   "Patch 1.4 Wiki",
				Snippet: "Community compiled patch details",
				URL:     "https://wutheringwaves.fandom.com/wiki/Patch_1.4",
			},
			{
				Title:   "Patch 1.4 Discussion",
				Snippet: "Community thoughts on the patch",
				URL:     "https://reddit.com/r/WutheringWaves/patch-1.4",
			},
		},
	}

	rag := &fakeRAGPort{chunks: nil}

	summarizer := &Summarizer{
		Search:     search,
		RAG:        rag,
		MaxBullets: 5,
	}

	summary, err := summarizer.Summarize(context.Background(), "patch 1.4")
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if summary.Query != "patch 1.4" {
		t.Errorf("Query = %q, want %q", summary.Query, "patch 1.4")
	}

	if len(summary.Bullets) != 3 {
		t.Errorf("len(Bullets) = %d, want 3", len(summary.Bullets))
	}

	if summary.Bullets[0].Source != LevelOfficial {
		t.Errorf("Bullets[0].Source = %v, want %v", summary.Bullets[0].Source, LevelOfficial)
	}
	if summary.Bullets[0].Label != "Resmi" {
		t.Errorf("Bullets[0].Label = %q, want %q", summary.Bullets[0].Label, "Resmi")
	}

	if summary.Bullets[1].Source != LevelWiki {
		t.Errorf("Bullets[1].Source = %v, want %v", summary.Bullets[1].Source, LevelWiki)
	}
	if summary.Bullets[1].Label != "Wiki" {
		t.Errorf("Bullets[1].Label = %q, want %q", summary.Bullets[1].Label, "Wiki")
	}

	if summary.Bullets[2].Source != LevelCommunity {
		t.Errorf("Bullets[2].Source = %v, want %v", summary.Bullets[2].Source, LevelCommunity)
	}
	if summary.Bullets[2].Label != "Komunitas" {
		t.Errorf("Bullets[2].Label = %q, want %q", summary.Bullets[2].Label, "Komunitas")
	}

	if summary.Note != "" {
		t.Errorf("Note = %q, want empty", summary.Note)
	}
}

func TestSummarizeRumorsOnlyAddsCaveat(t *testing.T) {
	search := &fakeSearchPort{
		results: []websearchpkg.SearchResult{
			{
				Title:   "Patch 1.4 Leak",
				Snippet: "Rumored changes from datamining",
				URL:     "https://reddit.com/r/WutheringWaves/leak",
			},
			{
				Title:   "Patch 1.4 Speculation",
				Snippet: "Community speculation on upcoming patch",
				URL:     "https://x.com/user/patch-speculation",
			},
		},
	}

	rag := &fakeRAGPort{chunks: nil}

	summarizer := &Summarizer{
		Search:     search,
		RAG:        rag,
		MaxBullets: 5,
	}

	summary, err := summarizer.Summarize(context.Background(), "patch 1.4 leak")
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if len(summary.Bullets) != 2 {
		t.Errorf("len(Bullets) = %d, want 2", len(summary.Bullets))
	}

	for i, bullet := range summary.Bullets {
		if bullet.Source != LevelCommunity {
			t.Errorf("Bullets[%d].Source = %v, want %v", i, bullet.Source, LevelCommunity)
		}
	}

	expectedNote := "Belum ada sumber resmi atau wiki yang terindeks. Informasi di bawah adalah rumor komunitas dan belum diverifikasi."
	if summary.Note != expectedNote {
		t.Errorf("Note = %q, want %q", summary.Note, expectedNote)
	}
}

func TestSummarizeMergesRAGChunks(t *testing.T) {
	search := &fakeSearchPort{
		results: []websearchpkg.SearchResult{
			{
				Title:   "Patch 1.4 Official",
				Snippet: "Official patch notes",
				URL:     "https://wutheringwaves.com/patch-1.4",
			},
		},
	}

	rag := &fakeRAGPort{
		chunks: []ragpkg.ScoredChunk{
			{
				Chunk: ragpkg.Chunk{
					Content: "New character Jinhsi released",
				},
				Score: 0.95,
			},
			{
				Chunk: ragpkg.Chunk{
					Content: "Balance changes to existing characters",
				},
				Score: 0.87,
			},
		},
	}

	summarizer := &Summarizer{
		Search:     search,
		RAG:        rag,
		MaxBullets: 5,
	}

	summary, err := summarizer.Summarize(context.Background(), "patch 1.4")
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if len(summary.Bullets) != 3 {
		t.Errorf("len(Bullets) = %d, want 3 (1 search + 2 RAG)", len(summary.Bullets))
	}

	if summary.Bullets[1].Source != LevelWiki {
		t.Errorf("Bullets[1].Source = %v, want %v (RAG chunk)", summary.Bullets[1].Source, LevelWiki)
	}
	if summary.Bullets[2].Source != LevelWiki {
		t.Errorf("Bullets[2].Source = %v, want %v (RAG chunk)", summary.Bullets[2].Source, LevelWiki)
	}
}

func TestSummarizeMaxBulletsClamped(t *testing.T) {
	search := &fakeSearchPort{
		results: []websearchpkg.SearchResult{
			{Title: "Result 1", Snippet: "Snippet 1", URL: "https://example.com/1"},
			{Title: "Result 2", Snippet: "Snippet 2", URL: "https://example.com/2"},
			{Title: "Result 3", Snippet: "Snippet 3", URL: "https://example.com/3"},
			{Title: "Result 4", Snippet: "Snippet 4", URL: "https://example.com/4"},
			{Title: "Result 5", Snippet: "Snippet 5", URL: "https://example.com/5"},
			{Title: "Result 6", Snippet: "Snippet 6", URL: "https://example.com/6"},
		},
	}

	rag := &fakeRAGPort{chunks: nil}

	summarizer := &Summarizer{
		Search:     search,
		RAG:        rag,
		MaxBullets: 3,
	}

	summary, err := summarizer.Summarize(context.Background(), "test")
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}

	if len(summary.Bullets) != 3 {
		t.Errorf("len(Bullets) = %d, want 3 (clamped)", len(summary.Bullets))
	}
}
