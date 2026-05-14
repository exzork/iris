package llm

import (
	"context"
	"fmt"
)

type Adapter struct {
	client *Client
}

func NewAdapter(cfg *Config) *Adapter {
	return &Adapter{
		client: NewClient(cfg),
	}
}

func (a *Adapter) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}

	return a.client.Chat(ctx, guildID, messages)
}

func (a *Adapter) CallTool(ctx context.Context, guildID int64, toolName string, arguments map[string]interface{}) (string, error) {
	if toolName == "" {
		return "", fmt.Errorf("tool name cannot be empty")
	}

	if arguments == nil {
		arguments = make(map[string]interface{})
	}

	return a.client.CallTool(ctx, guildID, toolName, arguments)
}
