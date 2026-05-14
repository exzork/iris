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

	contextEmbeddings := make([][]float32, 0, len(contextMessages))
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

		contextEmbeddings = append(contextEmbeddings, msgEmbedding)
	}

	if len(contextEmbeddings) == 0 {
		return false, 0, 0, nil
	}

	centroid := r.computeCentroid(contextEmbeddings)
	similarity := float64(embedder.Cosine(centroid, currentEmbedding))

	decision := similarity >= r.cfg.Threshold

	return decision, similarity, r.cfg.Threshold, nil
}

func (r *similarityInWindowRelevance) computeCentroid(embeddings [][]float32) []float32 {
	if len(embeddings) == 0 {
		return []float32{}
	}

	dim := len(embeddings[0])
	centroid := make([]float32, dim)

	for _, emb := range embeddings {
		for i := range centroid {
			centroid[i] += emb[i]
		}
	}

	for i := range centroid {
		centroid[i] /= float32(len(embeddings))
	}

	return embedder.L2Normalize(centroid)
}
