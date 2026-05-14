package orchestrator

import (
	"context"
	"log/slog"
	"sort"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/embedder"
)

const (
	defaultSimilarityThreshold     = 0.55
	defaultSimilarityMaxCandidates = 10
	defaultSimilarityWindowMinutes = 30
)

type SimilarityCrossChannelConfig struct {
	Store         CandidateStore
	Allowed       ChannelAllowQuerier
	Embedder      embedder.Embedder
	Threshold     float64
	MaxCandidates int
	WindowMinutes int
}

type similarityCrossChannelClassifier struct {
	cfg SimilarityCrossChannelConfig
}

func NewSimilarityCrossChannelClassifier(cfg SimilarityCrossChannelConfig) CrossChannelClassifier {
	if cfg.Threshold == 0 {
		cfg.Threshold = defaultSimilarityThreshold
	}
	if cfg.MaxCandidates <= 0 {
		cfg.MaxCandidates = defaultSimilarityMaxCandidates
	}
	if cfg.WindowMinutes <= 0 {
		cfg.WindowMinutes = defaultSimilarityWindowMinutes
	}
	return &similarityCrossChannelClassifier{cfg: cfg}
}

func (c *similarityCrossChannelClassifier) Classify(ctx context.Context, event *domain.DiscordEvent) ([]*domain.ChannelMessage, error) {
	if c == nil || c.cfg.Store == nil || c.cfg.Embedder == nil || event == nil || event.Message == nil {
		return nil, nil
	}

	raw, err := c.cfg.Store.ListByUserAcrossChannels(ctx, event.GuildID, event.UserID, c.cfg.WindowMinutes, crossChannelStoreQueryLimit)
	if err != nil {
		slog.Default().Warn("cross-channel-sim candidate query failed", "err", err)
		return nil, nil
	}

	hasAllowList := false
	if c.cfg.Allowed != nil {
		hasAny, allowErr := c.cfg.Allowed.HasAny(ctx, event.GuildID)
		if allowErr != nil {
			slog.Default().Warn("cross-channel-sim allow-list mode check failed", "err", allowErr)
			return nil, nil
		}
		hasAllowList = hasAny
	}

	filtered := make([]*domain.ChannelMessage, 0, len(raw))
	for _, msg := range raw {
		if msg == nil {
			continue
		}
		if msg.GuildID != event.GuildID {
			continue
		}
		if msg.ChannelID == event.ChannelID {
			continue
		}
		if msg.IsBot {
			continue
		}

		if hasAllowList {
			allowed, allowErr := c.cfg.Allowed.IsAllowed(ctx, event.GuildID, msg.ChannelID)
			if allowErr != nil {
				slog.Default().Warn("cross-channel-sim allow-list channel check failed", "channel_id", msg.ChannelID, "err", allowErr)
				return nil, nil
			}
			if !allowed {
				continue
			}
		}

		filtered = append(filtered, msg)
	}

	if len(filtered) == 0 {
		return nil, nil
	}

	currentVec, embedErr := c.cfg.Embedder.Embed(ctx, event.Message.Content)
	if embedErr != nil {
		slog.Default().Warn("cross-channel-sim current message embed failed", "err", embedErr)
		return nil, embedErr
	}

	type scoredCandidate struct {
		msg   *domain.ChannelMessage
		score float32
	}

	scored := make([]scoredCandidate, 0, len(filtered))

	for _, msg := range filtered {
		var candVec []float32

		if len(msg.ContentEmbedding) > 0 {
			candVec = msg.ContentEmbedding
		} else {
			embVec, embErr := c.cfg.Embedder.Embed(ctx, msg.Content)
			if embErr != nil {
				slog.Default().Debug("cross-channel-sim candidate embed failed, skipping", "message_id", msg.MessageID, "err", embErr)
				continue
			}
			candVec = embVec
		}

		sim := embedder.Cosine(currentVec, candVec)
		if sim >= float32(c.cfg.Threshold) {
			scored = append(scored, scoredCandidate{msg: msg, score: sim})
			slog.DebugContext(ctx, "cross_channel_sim_kept", "id", msg.MessageID, "sim", sim, "channel", msg.ChannelID)
		} else {
			slog.DebugContext(ctx, "cross_channel_sim_rejected", "id", msg.MessageID, "sim", sim, "channel", msg.ChannelID)
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if len(scored) > c.cfg.MaxCandidates {
		scored = scored[:c.cfg.MaxCandidates]
	}

	result := make([]*domain.ChannelMessage, len(scored))
	for i, sc := range scored {
		result[i] = sc.msg
	}

	slog.InfoContext(ctx, "cross_channel_classified", "guild", event.GuildID, "current", event.ChannelID, "kept", len(result))

	return result, nil
}
