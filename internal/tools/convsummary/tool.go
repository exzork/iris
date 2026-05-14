package convsummary

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/eko/iris-bot/internal/tools"
)

// Tool implements tools.Tool for conversation summarization.
type Tool struct {
	S *Summarizer
}

// Schema returns the tool schema.
func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "conversation_summarizer",
		Description: "Ringkas percakapan channel Discord dalam Bahasa Indonesia.",
		Fields: []tools.FieldSpec{
			{
				Name:        "guild_id",
				Kind:        tools.KindNumber,
				Required:    true,
				Description: "Discord guild ID",
			},
			{
				Name:        "channel_id",
				Kind:        tools.KindNumber,
				Required:    true,
				Description: "Discord channel ID",
			},
			{
				Name:        "limit",
				Kind:        tools.KindNumber,
				Required:    false,
				Description: "Maximum number of messages to summarize (default: 20)",
			},
		},
	}
}

// Run executes the conversation summarizer tool.
func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	schema := t.Schema()
	if err := schema.ValidateArgs(args); err != nil {
		return "", err
	}

	guildID := int64(args["guild_id"].(float64))
	channelID := int64(args["channel_id"].(float64))

	limit := 20
	if l, ok := args["limit"]; ok {
		limit = int(l.(float64))
	}

	summary, err := t.S.Summarize(ctx, guildID, channelID, limit)
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}

	result := map[string]interface{}{
		"empty":      summary.Empty,
		"text":       summary.Text,
		"channel_id": summary.ChannelID,
	}

	if !summary.Empty {
		result["from"] = summary.From.Format("2006-01-02T15:04:05Z07:00")
		result["to"] = summary.To.Format("2006-01-02T15:04:05Z07:00")
		result["count"] = summary.Count
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}

	return string(jsonBytes), nil
}
