package canoncheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/eko/iris-bot/internal/tools"
)

type Tool struct {
	Verifier *Verifier
}

func New(v *Verifier) *Tool {
	return &Tool{Verifier: v}
}

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        "canon_check",
		Description: "Verify a Wuthering Waves lore claim against indexed wiki sources. Returns status with citations.",
		Fields: []tools.FieldSpec{
			{
				Name:        "claim",
				Kind:        tools.KindString,
				Required:    true,
				Description: "The claim to verify",
			},
			{
				Name:        "query",
				Kind:        tools.KindString,
				Required:    false,
				Description: "Optional retrieval query hint",
			},
		},
	}
}

func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	claimText, ok := args["claim"].(string)
	if !ok || claimText == "" {
		return "", errors.New("claim is required and must be a non-empty string")
	}

	query, _ := args["query"].(string)

	claim := Claim{
		Text:  claimText,
		Query: query,
	}

	verdict, err := t.Verifier.Check(ctx, claim)
	if err != nil {
		return "", fmt.Errorf("verification failed: %w", err)
	}

	result := map[string]interface{}{
		"status":     verdict.Status,
		"confidence": verdict.Confidence,
		"reason":     verdict.Reason,
	}

	if len(verdict.Citations) > 0 {
		citations := make([]map[string]string, len(verdict.Citations))
		for i, c := range verdict.Citations {
			citations[i] = map[string]string{
				"title": c.Title,
				"url":   c.URL,
			}
		}
		result["citations"] = citations
	}

	if len(verdict.Snippets) > 0 {
		result["snippets"] = verdict.Snippets
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("json marshal failed: %w", err)
	}

	return string(jsonBytes), nil
}
