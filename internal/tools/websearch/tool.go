package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eko/iris-bot/internal/tools"
)

type Tool struct {
	Provider Provider
	Limit    int
}

func New(p Provider) *Tool {
	return &Tool{
		Provider: p,
		Limit:    5,
	}
}

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "web_search",
		Description: "Search the web for recent information. Not canon for Wuthering Waves unless source is authoritative.",
		Fields: []tools.FieldSpec{
			{
				Name:        "query",
				Kind:        tools.KindString,
				Required:    true,
				Description: "search query",
			},
			{
				Name:        "limit",
				Kind:        tools.KindNumber,
				Required:    false,
				Description: "max results 1-10",
			},
		},
	}
}

func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("query must be a string")
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return "", ErrEmptyQuery
	}

	limit := t.Limit
	if limitVal, ok := args["limit"]; ok {
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}

	if limit < 1 {
		limit = t.Limit
	}
	if limit > 10 {
		limit = 10
	}

	results, err := t.Provider.Search(ctx, query, limit)
	if err != nil {
		return "", err
	}

	output := map[string]interface{}{
		"provider": t.Provider.Name(),
		"results": results,
	}

	jsonBytes, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("failed to marshal results: %w", err)
	}

	return string(jsonBytes), nil
}
