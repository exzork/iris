package memory

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/eko/iris-bot/internal/domain"
)

const (
	maxMemoryChars = 500
	defaultTopK    = 5
)

// EmbeddingProvider creates vectors from text.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// MemoryStore persists and retrieves guild-scoped memory rows.
type MemoryStore interface {
	Save(ctx context.Context, guildID int64, userID int64, content string, embedding []float32) error
	SearchSimilar(ctx context.Context, guildID int64, embedding []float32, limit int) ([]domain.MemoryRecord, error)
}

// MemoryService coordinates gate, redactor, embeddings, and store for selective long-term memory.
type MemoryService struct {
	gate     *Gate
	redactor *Redactor
	filter   *PersonaFilter
	embed    EmbeddingProvider
	store    MemoryStore
	topK     int
}

// Config for NewMemoryService.
type Config struct {
	Embed EmbeddingProvider
	Store MemoryStore
	TopK  int
}

func NewMemoryService(cfg Config) *MemoryService {
	topK := cfg.TopK
	if topK <= 0 {
		topK = defaultTopK
	}
	return &MemoryService{
		gate:     NewGate(),
		redactor: NewRedactor(),
		filter:   NewPersonaFilter(),
		embed:    cfg.Embed,
		store:    cfg.Store,
		topK:     topK,
	}
}

// Consider evaluates text and saves it if the gate and redactor allow.
// Returns true if a row was written.
func (s *MemoryService) Consider(ctx context.Context, guildID, userID int64, text string) (bool, error) {
	decision := s.gate.Decide(text)
	if !decision.Save {
		return false, nil
	}
	return s.store_(ctx, guildID, userID, text)
}

// Store saves text without consulting the keyword gate. Intended for callers
// that have already made an LLM-driven decision (e.g. orchestrator promoter)
// and just need the redact + embed + persist pipeline. Returns true if a row
// was written.
func (s *MemoryService) Store(ctx context.Context, guildID, userID int64, text string) (bool, error) {
	return s.store_(ctx, guildID, userID, text)
}

func (s *MemoryService) store_(ctx context.Context, guildID, userID int64, text string) (bool, error) {
	if s.redactor.IsFullyRedacted(text) {
		slog.Default().Warn("memory_service_redacted", "guild", guildID, "user", userID, "chars", len(text))
		return false, nil
	}

	cleaned := s.redactor.Redact(text)
	if utf8.RuneCountInString(cleaned) > maxMemoryChars {
		cleaned = truncateRunes(cleaned, maxMemoryChars)
	}

	vec, err := s.embed.Embed(ctx, cleaned)
	if err != nil {
		slog.Default().Warn("memory_service_embed_failed", "guild", guildID, "user", userID, "err", err.Error())
		return false, err
	}

	if err := s.store.Save(ctx, guildID, userID, cleaned, vec); err != nil {
		slog.Default().Warn("memory_service_save_failed", "guild", guildID, "user", userID, "err", err.Error())
		return false, err
	}
	slog.Default().Info("memory_service_saved", "guild", guildID, "user", userID, "chars", len(cleaned), "vec_dim", len(vec))
	return true, nil
}

// Retrieve returns memory rows relevant to the query.
// Returns nil (no error) when the query is a pure command/lore/tool request.
func (s *MemoryService) Retrieve(ctx context.Context, guildID int64, query string, limit int) ([]string, error) {
	if !s.shouldRetrieve(query) {
		return nil, nil
	}

	if limit <= 0 {
		limit = s.topK
	}

	vec, err := s.embed.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.SearchSimilar(ctx, guildID, vec, limit)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if !s.filter.IsSafe(row.Content) {
			continue
		}
		out = append(out, row.Content)
	}
	return out, nil
}

// AssemblePromptContext returns memory facts to be layered BELOW the immutable persona.
func (s *MemoryService) AssemblePromptContext(ctx context.Context, guildID int64, query string) ([]string, error) {
	return s.Retrieve(ctx, guildID, query, s.topK)
}

var personalMarkers = []string{
	"aku", "saya", "ku ", "my ", "me ", " i ", "i'm", "im ",
	"preferensi", "prefer", "like", "suka", "remember", "ingat",
}

func (s *MemoryService) shouldRetrieve(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "!") {
		return false
	}
	lower := " " + strings.ToLower(trimmed) + " "
	for _, marker := range personalMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
