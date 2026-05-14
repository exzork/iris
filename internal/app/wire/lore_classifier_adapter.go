package wire

import (
	"context"
	"fmt"

	"github.com/eko/iris-bot/internal/lorethread"
)

// LoreClassifierAdapter adapts the LLM client to the lorethread.LoreClassifier interface.
type LoreClassifierAdapter struct {
	Client LLMCaller
	Model  string
}

// LLMCaller is the interface for LLM calls.
type LLMCaller interface {
	ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error)
}

func (a *LoreClassifierAdapter) Classify(ctx context.Context, guildID int64, message *lorethread.Message) (*lorethread.ClassifyResult, error) {
	prompt := fmt.Sprintf(`You are a lore classifier. Determine if the following Discord message is lore-relevant (worldbuilding, character development, plot, setting, or narrative discussion).

Message: %q

Respond with ONLY "LORE" or "NOT_LORE".`, message.Content)

	messages := []map[string]string{
		{
			"role":    "user",
			"content": prompt,
		},
	}

	response, err := a.Client.ChatWithModel(ctx, a.Model, guildID, messages)
	if err != nil {
		return nil, err
	}

	isLore := response == "LORE"
	return &lorethread.ClassifyResult{
		IsLore: isLore,
		Reason: response,
	}, nil
}
