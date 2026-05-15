package orchestrator

import (
	"context"
	"errors"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
)

type SimilarityInWindowConfig struct {
	Embedder   embedder.Embedder
	Threshold  float64
	MinContext int
}

type similarityInWindowRelevance struct {
	cfg SimilarityInWindowConfig
}

func NewSimilarityInWindowRelevance(cfg SimilarityInWindowConfig) InWindowRelevance {
	if cfg.Threshold == 0 {
		cfg.Threshold = 0.55
	}
	if cfg.MinContext == 0 {
		cfg.MinContext = 1
	}
	return &similarityInWindowRelevance{cfg: cfg}
}

// IsRelevant decides whether the current message continues the in-window
// conversation by comparing its embedding against each context-message
// embedding individually and taking the best (max) cosine similarity. This
// matches the wiki_chunks retrieval path (pgvector `<=>` cosine distance,
// score = 1 - distance, top hit wins) so on-topic detection survives mixed
// context windows that would dilute a centroid-averaged comparison.
func (r *similarityInWindowRelevance) IsRelevant(ctx context.Context, event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage) (bool, float64, float64, error) {
	if r.cfg.Embedder == nil {
		return false, 0, 0, errors.New("embedder unavailable")
	}

	if len(contextMessages) < r.cfg.MinContext {
		return false, 0, 0, nil
	}

	currentEmbedding, err := r.cfg.Embedder.Embed(ctx, event.Message.Content)
	if err != nil {
		return false, 0, 0, err
	}

	bestSim := float64(-1)
	considered := 0
	for _, msg := range contextMessages {
		if msg == nil {
			continue
		}
		var msgEmbedding []float32
		if len(msg.ContentEmbedding) > 0 {
			msgEmbedding = msg.ContentEmbedding
		} else {
			embedded, err := r.cfg.Embedder.Embed(ctx, msg.Content)
			if err != nil {
				continue
			}
			msgEmbedding = embedded
		}
		sim := float64(embedder.Cosine(msgEmbedding, currentEmbedding))
		considered++
		if sim > bestSim {
			bestSim = sim
		}
	}

	if considered == 0 {
		return false, 0, 0, nil
	}

	decision := bestSim >= r.cfg.Threshold
	return decision, bestSim, r.cfg.Threshold, nil
}
