package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
)

type InWindowRelevance interface {
	IsRelevant(ctx context.Context, event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage) (bool, float64, float64, error)
}

type LLMInWindowRelevanceConfig struct {
	LLM     CrossChannelLLM
	Model   string
	Timeout time.Duration
}

type llmInWindowRelevance struct {
	cfg LLMInWindowRelevanceConfig
}

type relevanceResponse struct {
	InContext bool   `json:"in_context"`
	Reason    string `json:"reason"`
}

func NewLLMInWindowRelevance(cfg LLMInWindowRelevanceConfig) InWindowRelevance {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 4 * time.Second
	}
	return &llmInWindowRelevance{cfg: cfg}
}

func (r *llmInWindowRelevance) IsRelevant(ctx context.Context, event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage) (bool, float64, float64, error) {
	if len(contextMessages) == 0 {
		return false, 0, 0, nil
	}

	systemPrompt := `You are a relevance classifier for Discord conversations. 
Determine if the current message is relevant to the ongoing conversation context.
Reply with STRICT JSON: {"in_context": bool, "reason": string}`

	contextSummary := r.buildContextSummary(contextMessages)
	currentMessage := r.formatCurrentMessage(event)

	userMessage := fmt.Sprintf("Recent context:\n%s\n\nCurrent message:\n%s\n\nIs the current message relevant to the conversation?", contextSummary, currentMessage)

	messages := []map[string]string{
		{
			"role":    "system",
			"content": systemPrompt,
		},
		{
			"role":    "user",
			"content": userMessage,
		},
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
	defer cancel()

	meta := &llm.ContextMeta{
		TriggerReason: "conversation_lock_relevance",
		GuildID:       event.GuildID,
		ChannelID:     event.ChannelID,
		MessageID:     event.Message.ID,
	}
	timeoutCtx = llm.WithMeta(timeoutCtx, meta)

	response, err := r.cfg.LLM.ChatWithModel(timeoutCtx, r.cfg.Model, event.GuildID, messages)
	if err != nil {
		return false, 0, 0, err
	}

	var result relevanceResponse
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return false, 0, 0, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return result.InContext, 0, 0, nil
}

func (r *llmInWindowRelevance) buildContextSummary(contextMessages []*domain.ChannelMessage) string {
	var buf strings.Builder
	for _, msg := range contextMessages {
		if msg == nil {
			continue
		}
		userLabel := r.getUserLabel(msg.AuthorName, msg.UserID)
		timestamp := msg.CreatedAt.Format(time.RFC3339)
		fmt.Fprintf(&buf, "[%d · %s · %s] %s\n", msg.ChannelID, userLabel, timestamp, msg.Content)
	}
	return buf.String()
}

func (r *llmInWindowRelevance) formatCurrentMessage(event *domain.DiscordEvent) string {
	if event.Message == nil {
		return ""
	}
	userLabel := r.getUserLabel(event.AuthorName, event.UserID)
	timestamp := event.CreatedAt.Format(time.RFC3339)
	return fmt.Sprintf("[%d · %s · %s] %s", event.ChannelID, userLabel, timestamp, event.Message.Content)
}

func (r *llmInWindowRelevance) getUserLabel(authorName *string, userID int64) string {
	if authorName != nil && *authorName != "" {
		return *authorName
	}
	return fmt.Sprintf("user:%d", userID)
}

// Backward compatibility alias for T5 wiring
type InWindowRelevanceConfig = LLMInWindowRelevanceConfig

func NewInWindowRelevance(cfg LLMInWindowRelevanceConfig) InWindowRelevance {
	return NewLLMInWindowRelevance(cfg)
}
