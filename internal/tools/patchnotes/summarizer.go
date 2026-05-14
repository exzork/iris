package patchnotes

import (
	"context"
	"fmt"
	"strings"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
	websearchpkg "github.com/eko/iris-bot/internal/tools/websearch"
)

// Bullet represents a single summary point with source information.
type Bullet struct {
	Text   string      `json:"text"`
	URL    string      `json:"url"`
	Source SourceLevel `json:"source"`
	Label  string      `json:"label"`
}

// Summary represents the complete patch note summary.
type Summary struct {
	Query   string   `json:"query"`
	Bullets []Bullet `json:"bullets"`
	Note    string   `json:"note"`
}

// Summarizer composes patch note summaries from web search and RAG sources.
type Summarizer struct {
	Search     SearchPort
	RAG        RAGPort
	MaxBullets int
}

// Summarize retrieves and summarizes patch notes for the given query.
func (s *Summarizer) Summarize(ctx context.Context, query string) (*Summary, error) {
	if s.MaxBullets == 0 {
		s.MaxBullets = 5
	}

	searchResults, err := s.Search.Search(ctx, query, 8)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	ragChunks, err := s.RAG.Retrieve(ctx, query, 5)
	if err != nil {
		return nil, fmt.Errorf("rag retrieve failed: %w", err)
	}

	bullets := s.buildBullets(searchResults, ragChunks)

	if len(bullets) > s.MaxBullets {
		bullets = bullets[:s.MaxBullets]
	}

	note := ""
	if s.allCommunity(bullets) {
		note = "Belum ada sumber resmi atau wiki yang terindeks. Informasi di bawah adalah rumor komunitas dan belum diverifikasi."
	}

	return &Summary{
		Query:   query,
		Bullets: bullets,
		Note:    note,
	}, nil
}

func (s *Summarizer) buildBullets(searchResults []websearchpkg.SearchResult, ragChunks []ragpkg.ScoredChunk) []Bullet {
	var bullets []Bullet

	officialBullets := []Bullet{}
	wikiBullets := []Bullet{}
	communityBullets := []Bullet{}

	for _, result := range searchResults {
		level := ClassifySource(result.URL)
		text := truncateText(result.Title + ": " + result.Snippet)
		bullet := Bullet{
			Text:   text,
			URL:    result.URL,
			Source: level,
			Label:  level.Label(),
		}

		switch level {
		case LevelOfficial:
			officialBullets = append(officialBullets, bullet)
		case LevelWiki:
			wikiBullets = append(wikiBullets, bullet)
		case LevelCommunity:
			communityBullets = append(communityBullets, bullet)
		}
	}

	bullets = append(bullets, officialBullets...)
	bullets = append(bullets, wikiBullets...)
	bullets = append(bullets, communityBullets...)

	for _, chunk := range ragChunks {
		text := truncateText(chunk.Content)
		bullet := Bullet{
			Text:   text,
			URL:    "",
			Source: LevelWiki,
			Label:  LevelWiki.Label(),
		}
		bullets = append(bullets, bullet)
	}

	return bullets
}

func (s *Summarizer) allCommunity(bullets []Bullet) bool {
	if len(bullets) == 0 {
		return false
	}
	for _, b := range bullets {
		if b.Source != LevelCommunity {
			return false
		}
	}
	return true
}

func truncateText(text string) string {
	const maxLen = 200
	text = strings.TrimSpace(text)
	if len(text) > maxLen {
		text = text[:maxLen] + "..."
	}
	return text
}
