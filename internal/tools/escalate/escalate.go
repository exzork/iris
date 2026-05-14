package escalate

import (
	"context"
	"errors"

	"github.com/eko/iris-bot/internal/tools"
)

const Name = "escalate_to_strong_model"

const MarkerPrefix = "ESCALATED:"

type Tool struct{}

func New() *Tool { return &Tool{} }

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        Name,
		Description: "Call when the current model cannot answer reliably or the query needs deep reasoning. Triggers a re-run with a stronger model. Do NOT call for simple greetings or basic questions.",
		Fields: []tools.FieldSpec{
			{
				Name:        "reason",
				Kind:        tools.KindString,
				Required:    true,
				Description: "Short reason why a stronger model is needed (1 sentence).",
			},
		},
	}
}

func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	reason, ok := args["reason"].(string)
	if !ok || reason == "" {
		return "", errors.New("escalate_to_strong_model: missing required arg: reason")
	}
	return MarkerPrefix + reason, nil
}
