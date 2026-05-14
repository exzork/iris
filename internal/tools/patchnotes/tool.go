package patchnotes

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/eko/iris-bot/internal/tools"
)

type Tool struct {
	S *Summarizer
}

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "patch_summarizer",
		Description: "Summarize recent Wuthering Waves patch notes and news with source attribution.",
		Fields: []tools.FieldSpec{
			{
				Name:        "query",
				Kind:        tools.KindString,
				Required:    true,
				Description: "Search query for patch notes or news (e.g., 'patch 1.4 notes')",
			},
			{
				Name:        "max_bullets",
				Kind:        tools.KindNumber,
				Required:    false,
				Description: "Maximum number of summary bullets to return (default: 5)",
			},
		},
	}
}

func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("query must be a string")
	}

	maxBullets := 5
	if mb, ok := args["max_bullets"]; ok {
		switch v := mb.(type) {
		case float64:
			maxBullets = int(v)
		case int:
			maxBullets = v
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return "", fmt.Errorf("max_bullets must be a number")
			}
			maxBullets = parsed
		default:
			return "", fmt.Errorf("max_bullets must be a number")
		}
	}

	t.S.MaxBullets = maxBullets

	summary, err := t.S.Summarize(ctx, query)
	if err != nil {
		return "", fmt.Errorf("summarize failed: %w", err)
	}

	data, err := json.Marshal(summary)
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	return string(data), nil
}
