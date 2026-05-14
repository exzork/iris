package memory

import (
	"context"
	"errors"
	"strings"

	"github.com/eko/iris-bot/internal/embedder"
	"github.com/eko/iris-bot/internal/repository"
)

// ErrMissingGuildID is returned by GuildRecallService when a recall is
// attempted without a guild. Guild isolation is a hard contract.
var ErrMissingGuildID = errors.New("memory: guild_id is required")

type recallStore interface {
	RecallByVector(ctx context.Context, guildID int64, query []float32, threshold float64, topK int) ([]*repository.RecallResult, error)
}

// GuildRecallConfig controls the guild-shared vector recall path.
type GuildRecallConfig struct {
	Enabled   bool
	Threshold float64
	TopK      int
}

// GuildRecallService returns same-guild memories whose cosine similarity to
// the current query passes the configured threshold. It is the only caller
// path that downstream prompt assembly should use for server memory.
type GuildRecallService struct {
	embedder embedder.Embedder
	store    recallStore
	cfg      GuildRecallConfig
}

// NewGuildRecallService wires the service. embedder and store must be non-nil.
// Threshold is clamped to [0,1]; non-positive TopK degrades to "no recall".
func NewGuildRecallService(embed embedder.Embedder, store recallStore, cfg GuildRecallConfig) (*GuildRecallService, error) {
	if embed == nil {
		return nil, errors.New("guild recall: embedder is nil")
	}
	if store == nil {
		return nil, errors.New("guild recall: store is nil")
	}
	if embed.Dim() != repository.ExpectedEmbeddingDim {
		return nil, errors.New("guild recall: embedder dim mismatch")
	}
	if cfg.Threshold < 0 {
		cfg.Threshold = 0
	}
	if cfg.Threshold > 1 {
		cfg.Threshold = 1
	}
	return &GuildRecallService{embedder: embed, store: store, cfg: cfg}, nil
}

// Recall returns ranked same-guild memories for the given query text. It
// returns an empty slice (not nil error) when the service is disabled,
// the query is empty, or the embedder fails, so callers can still respond
// normally when memory is not available.
func (s *GuildRecallService) Recall(ctx context.Context, guildID int64, query string) ([]*repository.RecallResult, error) {
	if !s.cfg.Enabled {
		return nil, nil
	}
	if guildID == 0 {
		return nil, ErrMissingGuildID
	}
	if s.cfg.TopK <= 0 {
		return nil, nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, nil
	}
	return s.store.RecallByVector(ctx, guildID, vec, s.cfg.Threshold, s.cfg.TopK)
}
