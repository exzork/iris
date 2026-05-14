package domain

import "context"

// DiscordClient defines the contract for Discord interactions.
type DiscordClient interface {
	SendMessage(ctx context.Context, guildID, channelID int64, content string) error
	GetMessage(ctx context.Context, guildID, channelID, messageID int64) (*DiscordMessage, error)
	GetGuild(ctx context.Context, guildID int64) (*Guild, error)
}

// LLMClient defines the contract for LLM interactions.
type LLMClient interface {
	Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error)
	CallTool(ctx context.Context, guildID int64, toolName string, arguments map[string]interface{}) (string, error)
}

// EmbeddingClient defines the contract for embedding generation.
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// ImageClient defines the contract for image generation.
type ImageClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// StoragePort defines the contract for persistent storage.
type StoragePort interface {
	SaveMemory(ctx context.Context, guildID int64, content string, embedding []float32) error
	GetMemories(ctx context.Context, guildID int64, limit int) ([]MemoryRecord, error)
	SaveToolResult(ctx context.Context, result *ToolResult) error
	GetToolResult(ctx context.Context, toolRequestID string) (*ToolResult, error)
	SaveLoreCitation(ctx context.Context, citation *LoreCitation) error
	GetLoreCitations(ctx context.Context, guildID int64, limit int) ([]LoreCitation, error)
	SaveGuildConfig(ctx context.Context, config *GuildConfig) error
	GetGuildConfig(ctx context.Context, guildID int64, key string) (*GuildConfig, error)
	AddExceptionChannel(ctx context.Context, guildID, channelID int64) error
	IsExceptionChannel(ctx context.Context, guildID, channelID int64) (bool, error)
}

// ToolExecutor defines the contract for tool execution.
type ToolExecutor interface {
	Execute(ctx context.Context, request *ToolRequest) (*ToolResult, error)
}

// RetrievalPort defines the contract for lore retrieval and indexing.
type RetrievalPort interface {
	IndexLore(ctx context.Context, guildID int64, content string, source string) error
	RetrieveLore(ctx context.Context, guildID int64, query string, limit int) ([]LoreCitation, error)
}
