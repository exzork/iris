package app

import (
	"context"

	"github.com/eko/iris-bot/internal/domain"
	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
	"github.com/eko/iris-bot/internal/router"
)

type TriggerPort interface {
	Decide(ctx context.Context, event *domain.DiscordEvent) (*router.Decision, error)
}

type MemoryPort interface {
	AssemblePromptContext(ctx context.Context, guildID int64, query string) ([]string, error)
	Consider(ctx context.Context, guildID, userID int64, text string) (bool, error)
}

type LorePort interface {
	Compose(ctx context.Context, query string) (*ragpkg.PromptContext, *ragpkg.UnsupportedResponse, error)
}

type LLMPort interface {
	Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error)
	ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error)
}

type ImagePort interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

type SenderPort interface {
	Send(ctx context.Context, guildID, channelID int64, content string) error
}
