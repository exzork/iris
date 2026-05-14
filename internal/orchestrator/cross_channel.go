package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
)

const (
	defaultCrossChannelTimeout      = 4 * time.Second
	defaultCrossChannelMaxCandidates = 10
	defaultCrossChannelWindowMinutes = 30
	crossChannelStoreQueryLimit      = 20
)

type CandidateStore interface {
	ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error)
}

type ChannelAllowQuerier interface {
	HasAny(ctx context.Context, guildID int64) (bool, error)
	IsAllowed(ctx context.Context, guildID int64, channelID int64) (bool, error)
}

type CrossChannelLLM interface {
	ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error)
}

type LLMCrossChannelConfig struct {
	Store         CandidateStore
	Allowed       ChannelAllowQuerier
	LLM           CrossChannelLLM
	Model         string
	Timeout       time.Duration
	MaxCandidates int
	WindowMinutes int
}

type CrossChannelClassifier interface {
	Classify(ctx context.Context, event *domain.DiscordEvent) ([]*domain.ChannelMessage, error)
}

type llmCrossChannelClassifier struct {
	cfg LLMCrossChannelConfig
}

func NewLLMCrossChannelClassifier(cfg LLMCrossChannelConfig) CrossChannelClassifier {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultCrossChannelTimeout
	}
	if cfg.MaxCandidates <= 0 {
		cfg.MaxCandidates = defaultCrossChannelMaxCandidates
	}
	if cfg.WindowMinutes <= 0 {
		cfg.WindowMinutes = defaultCrossChannelWindowMinutes
	}
	return &llmCrossChannelClassifier{cfg: cfg}
}

func (c *llmCrossChannelClassifier) Classify(ctx context.Context, event *domain.DiscordEvent) ([]*domain.ChannelMessage, error) {
	if c == nil || c.cfg.Store == nil || c.cfg.LLM == nil || event == nil || event.Message == nil {
		return nil, nil
	}

	raw, err := c.cfg.Store.ListByUserAcrossChannels(ctx, event.GuildID, event.UserID, c.cfg.WindowMinutes, crossChannelStoreQueryLimit)
	if err != nil {
		slog.Default().Warn("cross-channel candidate query failed", "err", err)
		return nil, nil
	}

	hasAllowList := false
	if c.cfg.Allowed != nil {
		hasAny, allowErr := c.cfg.Allowed.HasAny(ctx, event.GuildID)
		if allowErr != nil {
			slog.Default().Warn("cross-channel allow-list mode check failed", "err", allowErr)
			return nil, nil
		}
		hasAllowList = hasAny
	}

	filtered := make([]*domain.ChannelMessage, 0, c.cfg.MaxCandidates)
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
				slog.Default().Warn("cross-channel allow-list channel check failed", "channel_id", msg.ChannelID, "err", allowErr)
				return nil, nil
			}
			if !allowed {
				continue
			}
		}

		filtered = append(filtered, msg)
		if len(filtered) >= c.cfg.MaxCandidates {
			break
		}
	}

	if len(filtered) == 0 {
		return nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	metaCtx := llm.WithMeta(timeoutCtx, &llm.ContextMeta{
		GuildID:       event.GuildID,
		ChannelID:     event.ChannelID,
		MessageID:     event.Message.ID,
		UserID:        event.UserID,
		TriggerReason: "cross_channel_classify",
	})

	promptPayload, payloadErr := c.buildPromptPayload(event, filtered)
	if payloadErr != nil {
		slog.Default().Warn("cross-channel payload build failed", "err", payloadErr)
		return nil, nil
	}

	modelOutput, llmErr := c.cfg.LLM.ChatWithModel(metaCtx, c.cfg.Model, event.GuildID, []map[string]string{
		{
			"role": "system",
			"content": "You are a binary classifier. Return strict JSON only with schema: {\"merge\":true|false,\"reason\":\"short\"}. No markdown, no extra text.",
		},
		{
			"role": "user",
			"content": promptPayload,
		},
	})
	if llmErr != nil {
		slog.Default().Warn("cross-channel classifier call failed", "err", llmErr)
		return nil, nil
	}

	var decision struct {
		Merge  bool   `json:"merge"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(modelOutput)), &decision); err != nil {
		slog.Default().Warn("cross-channel classifier parse failed", "err", err)
		return nil, nil
	}

	if !decision.Merge {
		return nil, nil
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})

	return filtered, nil
}

func (c *llmCrossChannelClassifier) buildPromptPayload(event *domain.DiscordEvent, candidates []*domain.ChannelMessage) (string, error) {
	type candidateLine struct {
		ChannelID int64  `json:"channel_id"`
		MessageID int64  `json:"message_id"`
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}

	payload := struct {
		CurrentMessage string          `json:"current_message"`
		Candidates     []candidateLine `json:"candidates"`
	}{
		CurrentMessage: truncateRunes(event.Message.Content, 500),
		Candidates:     make([]candidateLine, 0, len(candidates)),
	}

	for _, msg := range candidates {
		payload.Candidates = append(payload.Candidates, candidateLine{
			ChannelID: msg.ChannelID,
			MessageID: msg.MessageID,
			Content:   truncateRunes(msg.Content, 220),
			CreatedAt: msg.CreatedAt.Format(time.RFC3339),
		})
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

// Backward compatibility alias for T5 wiring
type CrossChannelConfig = LLMCrossChannelConfig

func NewCrossChannelClassifier(cfg LLMCrossChannelConfig) CrossChannelClassifier {
	return NewLLMCrossChannelClassifier(cfg)
}
