package convsummary

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LLMSummarizer generates summaries from prompts.
type LLMSummarizer interface {
	Summarize(ctx context.Context, prompt string) (string, error)
}

// Summary represents a conversation summary.
type Summary struct {
	GuildID   int64
	ChannelID int64
	From      time.Time
	To        time.Time
	Count     int
	Text      string
	Empty     bool
}

// Summarizer orchestrates message fetching, redaction, and LLM summarization.
type Summarizer struct {
	History HistoryStore
	Redact  *Redactor
	LLM     LLMSummarizer
}

// Summarize fetches messages, redacts them, and generates a summary.
func (s *Summarizer) Summarize(ctx context.Context, guildID, channelID int64, limit int) (*Summary, error) {
	if limit <= 0 {
		limit = 20
	}

	msgs, err := s.History.Fetch(ctx, guildID, channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch messages: %w", err)
	}

	if len(msgs) == 0 {
		return &Summary{
			GuildID:   guildID,
			ChannelID: channelID,
			Empty:     true,
			Text:      "Belum ada riwayat pesan yang dapat diringkas untuk channel ini.",
		}, nil
	}

	redacted := s.Redact.RedactMessages(msgs)

	prompt := buildPrompt(redacted)

	llmResult, err := s.LLM.Summarize(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm summarize: %w", err)
	}

	return &Summary{
		GuildID:   guildID,
		ChannelID: channelID,
		From:      msgs[0].CreatedAt,
		To:        msgs[len(msgs)-1].CreatedAt,
		Count:     len(msgs),
		Text:      llmResult,
		Empty:     false,
	}, nil
}

func buildPrompt(msgs []Message) string {
	var sb strings.Builder
	sb.WriteString("Ringkas singkat percakapan berikut dalam Bahasa Indonesia (bullet list):\n")
	for _, msg := range msgs {
		sb.WriteString(msg.Username)
		sb.WriteString(": ")
		sb.WriteString(msg.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}
