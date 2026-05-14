package orchestrator

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/safety"
)

const (
	defaultMemoryPromotionTimeout = 15 * time.Second
	maxMemoryPromotionSummary     = 500
)

type MemoryWriter interface {
	Save(ctx context.Context, guildID int64, userID int64, content string) error
}

type SafetyChecker interface {
	IsSafeForMemory(content string) bool
}

type MemoryPromoterConfig struct {
	LLM     CrossChannelLLM
	Model   string
	Writer  MemoryWriter
	Safety  SafetyChecker
	Timeout time.Duration
}

type MemoryPromoter interface {
	Consider(ctx context.Context, event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage, response string)
}

type memoryPromoter struct {
	cfg MemoryPromoterConfig
}

type defaultSafetyChecker struct {
	injection *safety.InjectionFilter
	output    *safety.OutputFilter
}

func NewMemoryPromoter(cfg MemoryPromoterConfig) MemoryPromoter {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultMemoryPromotionTimeout
	}
	if cfg.Safety == nil {
		cfg.Safety = &defaultSafetyChecker{
			injection: safety.NewInjectionFilter(),
			output:    safety.NewOutputFilter(),
		}
	}
	return &memoryPromoter{cfg: cfg}
}

func (p *memoryPromoter) Consider(ctx context.Context, event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage, response string) {
	if p == nil || p.cfg.LLM == nil || p.cfg.Writer == nil || event == nil || event.Message == nil {
		return
	}

	go func() {
		detachedCtx, cancel := context.WithTimeout(context.Background(), p.cfg.Timeout)
		defer cancel()

		if ctx != nil {
			go func() {
				select {
				case <-ctx.Done():
					cancel()
				case <-detachedCtx.Done():
				}
			}()
		}

		metaCtx := llm.WithMeta(detachedCtx, &llm.ContextMeta{
			GuildID:       event.GuildID,
			ChannelID:     event.ChannelID,
			MessageID:     event.Message.ID,
			UserID:        event.UserID,
			TriggerReason: "memory_promotion",
		})

		payload, err := p.buildPromptPayload(event, contextMessages, response)
		if err != nil {
			slog.Default().Warn("memory promotion payload build failed", "err", err)
			return
		}

		out, err := p.cfg.LLM.ChatWithModel(metaCtx, p.cfg.Model, event.GuildID, []map[string]string{
			{
				"role": "system",
				"content": "You decide whether to store durable memory. Return strict JSON only with schema: {\"store\":true|false,\"summary\":\"<=500 chars\",\"reason\":\"short\"}. No markdown, no extra text.",
			},
			{
				"role": "user",
				"content": payload,
			},
		})
		if err != nil {
			slog.Default().Warn("memory promotion classifier failed", "err", err)
			return
		}

		var decision struct {
			Store   bool   `json:"store"`
			Summary string `json:"summary"`
			Reason  string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &decision); err != nil {
			return
		}
		if !decision.Store {
			return
		}

		summary := strings.TrimSpace(truncateRunes(decision.Summary, maxMemoryPromotionSummary))
		if summary == "" {
			return
		}

		if p.cfg.Safety != nil && !p.cfg.Safety.IsSafeForMemory(summary) {
			return
		}

		if err := p.cfg.Writer.Save(detachedCtx, event.GuildID, event.UserID, summary); err != nil {
			slog.Default().Warn("memory promotion write failed", "guild_id", event.GuildID, "user_id", event.UserID, "summary_chars", utf8.RuneCountInString(summary), "err", err)
		}
	}()
}

func (p *memoryPromoter) buildPromptPayload(event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage, response string) (string, error) {
	type contextLine struct {
		ChannelID int64  `json:"channel_id"`
		UserID    int64  `json:"user_id"`
		Content   string `json:"content"`
	}

	payload := struct {
		CurrentMessage string        `json:"current_message"`
		Response       string        `json:"response"`
		Context        []contextLine `json:"context"`
	}{
		CurrentMessage: truncateRunes(event.Message.Content, 500),
		Response:       truncateRunes(response, 700),
		Context:        make([]contextLine, 0, len(contextMessages)),
	}

	for _, msg := range contextMessages {
		if msg == nil {
			continue
		}
		payload.Context = append(payload.Context, contextLine{
			ChannelID: msg.ChannelID,
			UserID:    msg.UserID,
			Content:   truncateRunes(msg.Content, 220),
		})
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (s *defaultSafetyChecker) IsSafeForMemory(content string) bool {
	if s == nil {
		return false
	}
	if s.injection == nil {
		s.injection = safety.NewInjectionFilter()
	}
	if s.output == nil {
		s.output = safety.NewOutputFilter()
	}

	if len(s.injection.Detect(content)) > 0 {
		return false
	}
	filtered := s.output.Apply(content)
	if filtered.Blocked {
		return false
	}
	return strings.TrimSpace(filtered.Content) != ""
}
