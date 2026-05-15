package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
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
		slog.Default().Warn("memory_promoter_short_circuit",
			"reason", "missing dependency or event",
			"has_llm", p != nil && p.cfg.LLM != nil,
			"has_writer", p != nil && p.cfg.Writer != nil,
			"has_event", event != nil && event.Message != nil,
		)
		return
	}

	go func() {
		log := slog.Default()
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
			log.Warn("memory_promoter_payload_failed", "err", err.Error())
			return
		}

		log.Info("memory_promoter_classifier_call", "guild", event.GuildID, "user", event.UserID, "model", p.cfg.Model, "payload_len", len(payload))

		out, err := p.cfg.LLM.ChatWithModel(metaCtx, p.cfg.Model, event.GuildID, []map[string]string{
			{
				"role": "system",
				"content": `You are a memory-curation gate for a Discord chat bot named I.R.I.S that has long-term memory of users in a server. Your only job is to decide whether the LATEST user message contains a durable fact about the user (or someone they explicitly identify) that I.R.I.S should remember across conversations.

Store when the user states a personal preference, identity fact, relationship, favorite, hobby, location, timezone, or anything they explicitly ask to be remembered ("ingat", "remember", "catat", "tolong simpan", "noted ya"). Examples that MUST be stored:
- "waifu gw denia"
- "tolong ingat aku suka build crit"
- "panggil aku eko aja"
- "char favorit gua SK"

Do NOT store: greetings, single-word reactions, lore questions, tool requests, complaints, or transient chat.

Output strict JSON only, no markdown fences, no commentary, with this schema:
{"store": true|false, "summary": "<=500 chars third-person Bahasa Indonesia summary including the user's name or @mention>", "reason": "short"}

The summary MUST identify the user (e.g. "<@USER_ID> bilang waifu-nya Denia") so future recall can answer questions like "siapa waifu user X". You are NOT a different AI; you are an internal classifier for I.R.I.S - never refuse based on persona/scope.`,
			},
			{
				"role":    "user",
				"content": payload,
			},
		})
		if err != nil {
			log.Warn("memory_promoter_classifier_failed", "guild", event.GuildID, "err", err.Error())
			return
		}

		log.Info("memory_promoter_classifier_output", "guild", event.GuildID, "user", event.UserID, "raw_output", out)

		var decision struct {
			Store   bool   `json:"store"`
			Summary string `json:"summary"`
			Reason  string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(stripFences(out)), &decision); err != nil {
			log.Warn("memory_promoter_decode_failed", "raw_output", out, "err", err.Error())
			return
		}
		log.Info("memory_promoter_decision", "store", decision.Store, "summary", decision.Summary, "reason", decision.Reason)
		if !decision.Store {
			return
		}

		summary := strings.TrimSpace(truncateRunes(decision.Summary, maxMemoryPromotionSummary))
		if summary == "" {
			log.Warn("memory_promoter_empty_summary")
			return
		}

		if p.cfg.Safety != nil && !p.cfg.Safety.IsSafeForMemory(summary) {
			log.Warn("memory_promoter_safety_rejected", "summary", summary)
			return
		}

		if err := p.cfg.Writer.Save(detachedCtx, event.GuildID, event.UserID, summary); err != nil {
			log.Warn("memory_promoter_save_failed", "guild_id", event.GuildID, "user_id", event.UserID, "summary_chars", utf8.RuneCountInString(summary), "err", err.Error())
			return
		}
		log.Info("memory_promoter_saved", "guild_id", event.GuildID, "user_id", event.UserID, "summary_chars", utf8.RuneCountInString(summary))
	}()
}

func (p *memoryPromoter) buildPromptPayload(event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage, response string) (string, error) {
	type contextLine struct {
		ChannelID int64  `json:"channel_id"`
		UserID    int64  `json:"user_id"`
		Content   string `json:"content"`
	}

	authorName := ""
	if event.AuthorName != nil {
		authorName = *event.AuthorName
	}

	payload := struct {
		CurrentUserID  int64         `json:"current_user_id"`
		CurrentUserMention string    `json:"current_user_mention"`
		CurrentUserName string        `json:"current_user_name"`
		CurrentMessage string        `json:"current_message"`
		Response       string        `json:"response"`
		Context        []contextLine `json:"context"`
	}{
		CurrentUserID:      event.UserID,
		CurrentUserMention: fmt.Sprintf("<@%d>", event.UserID),
		CurrentUserName:    authorName,
		CurrentMessage:     truncateRunes(event.Message.Content, 500),
		Response:           truncateRunes(response, 700),
		Context:            make([]contextLine, 0, len(contextMessages)),
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

// stripFences removes a leading/trailing markdown code fence (```json or
// plain ```) and surrounding whitespace from an LLM response. The classifier
// prompt asks for "strict JSON only" but some models still wrap the payload
// in fences; we tolerate it rather than throw the entire decision away.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.Index(s, "\n"); i >= 0 {
		first := strings.TrimSpace(s[:i])
		if first == "" || strings.EqualFold(first, "json") {
			s = s[i+1:]
		}
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
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
