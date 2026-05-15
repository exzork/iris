package wire

import (
	"context"
	"regexp"
	"strings"

	"github.com/eko/iris-bot/internal/lore/ingest"
	"github.com/eko/iris-bot/internal/lore/rag"
	"github.com/eko/iris-bot/internal/orchestrator"
	"github.com/eko/iris-bot/internal/repository"
)

type WikiCursorAdapter struct {
	Repo     *repository.WikiRepo
	SourceID string
}

func (a *WikiCursorAdapter) Get(ctx context.Context, sourceID string) (*ingest.Cursor, error) {
	rec, err := a.Repo.GetCursor(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return &ingest.Cursor{
		SourceID:  rec.SourceID,
		LastID:    rec.LastPageID,
		LastTitle: rec.LastTitle,
		UpdatedAt: rec.UpdatedAt,
	}, nil
}

func (a *WikiCursorAdapter) Save(ctx context.Context, cur *ingest.Cursor) error {
	if cur == nil {
		return nil
	}
	return a.Repo.SaveCursor(ctx, &repository.WikiCursor{
		SourceID:   cur.SourceID,
		LastTitle:  cur.LastTitle,
		LastPageID: cur.LastID,
		UpdatedAt:  cur.UpdatedAt,
	})
}

type WikiDedupeAdapter struct {
	Repo     *repository.WikiRepo
	SourceID string
}

func (a *WikiDedupeAdapter) SeenHash(ctx context.Context, hash string) (bool, error) {
	return a.Repo.HashSeen(ctx, a.SourceID, hash)
}

func (a *WikiDedupeAdapter) MarkHash(ctx context.Context, hash string) error {
	return a.Repo.MarkHash(ctx, a.SourceID, hash)
}

type WikiIngestStoreAdapter struct {
	Repo     *repository.WikiRepo
	SourceID string
}

func (a *WikiIngestStoreAdapter) ChunkExists(ctx context.Context, pageID int64, chunkIndex int) (bool, error) {
	return a.Repo.ChunkExists(ctx, a.SourceID, pageID, chunkIndex)
}

func (a *WikiIngestStoreAdapter) InsertChunk(ctx context.Context, chunk ingest.LoreChunkRecord) error {
	if err := a.Repo.UpsertPage(ctx, a.SourceID, chunk.PageID, chunk.Title, chunk.URL, 0); err != nil {
		return err
	}
	return a.Repo.InsertChunk(
		ctx,
		a.SourceID,
		chunk.PageID,
		chunk.ChunkIdx,
		chunk.Content,
		ingest.ContentHash(chunk.Content),
		chunk.Title,
		chunk.URL,
		chunk.Embedding,
	)
}

type WikiRetrievalAdapter struct {
	Repo     *repository.WikiRepo
	SourceID string
}

func (a *WikiRetrievalAdapter) SearchSimilar(ctx context.Context, embedding []float32, topK int) ([]rag.ScoredChunk, error) {
	results, err := a.Repo.SearchSimilar(ctx, a.SourceID, embedding, topK)
	if err != nil {
		return nil, err
	}
	out := make([]rag.ScoredChunk, 0, len(results))
	for _, r := range results {
		out = append(out, rag.ScoredChunk{
			Chunk: rag.Chunk{
				ID:      r.ID,
				PageID:  r.PageID,
				Title:   r.Title,
				URL:     r.URL,
				Content: r.Content,
			},
			Score: 1 - r.Distance,
		})
	}
	return out, nil
}

type WikiLoreContextAdapter struct {
	Retriever *rag.Retriever
	TopK      int

	// MinTopScore drops the entire result set if the best snippet's score
	// is below this threshold. Cosine similarity from the ONNX MiniLM
	// embedder lands ~0.45-0.55 on direct topic matches and ~0.30-0.38 on
	// noise. Default 0.40 if unset.
	MinTopScore float64
}

var (
	reBotMention    = regexp.MustCompile(`<@!?\d+>`)
	reBotKeyword    = regexp.MustCompile(`(?i)\biris\b[\s,:.!?]*`)
	reLeadingPunct  = regexp.MustCompile(`^[\s,:.!?]+`)
	reTrailingPunct = regexp.MustCompile(`[\s,:.!?]+$`)
	reCollapseWS    = regexp.MustCompile(`\s+`)
)

// sanitizeQueryForEmbedding strips Discord mentions and the "iris" trigger
// keyword from the user query before embedding. The bot keyword pulls the
// vector toward unrelated wiki pages that happen to mention "iris" or
// punctuation; the LLM still sees the original message in the prompt.
func sanitizeQueryForEmbedding(query string) string {
	q := reBotMention.ReplaceAllString(query, " ")
	q = reBotKeyword.ReplaceAllString(q, " ")
	q = reLeadingPunct.ReplaceAllString(q, "")
	q = reTrailingPunct.ReplaceAllString(q, "")
	q = reCollapseWS.ReplaceAllString(q, " ")
	return strings.TrimSpace(q)
}

func (a *WikiLoreContextAdapter) LoreContext(ctx context.Context, query string) ([]orchestrator.LoreSnippet, []orchestrator.LoreCitation, error) {
	if a.Retriever == nil {
		return nil, nil, nil
	}
	cleaned := sanitizeQueryForEmbedding(query)
	if cleaned == "" {
		return nil, nil, nil
	}

	topK := a.TopK
	if topK <= 0 {
		topK = 5
	}
	threshold := a.MinTopScore
	if threshold <= 0 {
		threshold = 0.40
	}

	chunks, err := a.Retriever.Retrieve(ctx, cleaned, topK)
	if err != nil {
		return nil, nil, err
	}
	if len(chunks) == 0 {
		return nil, nil, nil
	}
	if chunks[0].Score < threshold {
		return nil, nil, nil
	}

	snippets := make([]orchestrator.LoreSnippet, 0, len(chunks))
	citationMap := make(map[string]orchestrator.LoreCitation)
	citationOrder := make([]string, 0, len(chunks))
	for _, c := range chunks {
		snippets = append(snippets, orchestrator.LoreSnippet{
			Title: c.Title,
			URL:   c.URL,
			Score: c.Score,
			Text:  c.Content,
		})
		if _, exists := citationMap[c.URL]; !exists {
			citationMap[c.URL] = orchestrator.LoreCitation{Title: c.Title, URL: c.URL}
			citationOrder = append(citationOrder, c.URL)
		}
	}
	citations := make([]orchestrator.LoreCitation, 0, len(citationOrder))
	for _, url := range citationOrder {
		citations = append(citations, citationMap[url])
	}
	return snippets, citations, nil
}
