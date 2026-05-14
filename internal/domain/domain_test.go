package domain

import (
	"context"
	"testing"
)

// TestGuildConfigContract verifies GuildConfig structure.
func TestGuildConfigContract(t *testing.T) {
	config := &GuildConfig{
		ID:           1,
		GuildID:      123456789,
		SettingKey:   "exception_channels",
		SettingValue: "channel1,channel2",
	}

	if config.GuildID == 0 {
		t.Error("GuildConfig must have non-zero GuildID")
	}
	if config.SettingKey == "" {
		t.Error("GuildConfig must have SettingKey")
	}
}

// TestMemoryRecordContract verifies MemoryRecord structure.
func TestMemoryRecordContract(t *testing.T) {
	record := &MemoryRecord{
		ID:      1,
		GuildID: 123456789,
		Content: "User prefers Indonesian responses",
		Embedding: []float32{0.1, 0.2, 0.3},
	}

	if record.GuildID == 0 {
		t.Error("MemoryRecord must have non-zero GuildID")
	}
	if record.Content == "" {
		t.Error("MemoryRecord must have Content")
	}
	if len(record.Embedding) == 0 {
		t.Error("MemoryRecord must have Embedding")
	}
}

// TestToolRequestValidation verifies ToolRequest validation.
func TestToolRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		request *ToolRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &ToolRequest{
				ID:       "req-123",
				ToolName: "web_search",
				Arguments: map[string]interface{}{"query": "Wuthering Waves lore"},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			request: &ToolRequest{
				ToolName: "web_search",
			},
			wantErr: true,
		},
		{
			name: "missing tool name",
			request: &ToolRequest{
				ID: "req-123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestToolResultValidation verifies ToolResult validation.
func TestToolResultValidation(t *testing.T) {
	tests := []struct {
		name    string
		result  *ToolResult
		wantErr bool
	}{
		{
			name: "valid result",
			result: &ToolResult{
				ID:       "req-123",
				ToolName: "web_search",
				Output:   "Search results...",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			result: &ToolResult{
				ToolName: "web_search",
				Output:   "Search results...",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLoreCitationContract verifies LoreCitation structure.
func TestLoreCitationContract(t *testing.T) {
	citation := &LoreCitation{
		ID:      1,
		GuildID: 123456789,
		Source:  "Wuthering Waves Wiki",
		Content: "I.R.I.S is a retrieval specialist...",
		URL:     "https://wutheringwaves.fandom.com/wiki/I.R.I.S",
	}

	if citation.GuildID == 0 {
		t.Error("LoreCitation must have non-zero GuildID")
	}
	if citation.Source == "" {
		t.Error("LoreCitation must have Source")
	}
	if citation.URL == "" {
		t.Error("LoreCitation must have URL")
	}
}

// TestDiscordMessageContract verifies DiscordMessage structure.
func TestDiscordMessageContract(t *testing.T) {
	msg := &DiscordMessage{
		ID:        987654321,
		GuildID:   123456789,
		ChannelID: 111111111,
		UserID:    222222222,
		Content:   "Hello I.R.I.S",
	}

	if msg.GuildID == 0 {
		t.Error("DiscordMessage must have non-zero GuildID")
	}
	if msg.ChannelID == 0 {
		t.Error("DiscordMessage must have non-zero ChannelID")
	}
	if msg.UserID == 0 {
		t.Error("DiscordMessage must have non-zero UserID")
	}
}

// TestDiscordEventContract verifies DiscordEvent structure.
func TestDiscordEventContract(t *testing.T) {
	event := &DiscordEvent{
		Type:      "message_create",
		GuildID:   123456789,
		ChannelID: 111111111,
		UserID:    222222222,
	}

	if event.Type == "" {
		t.Error("DiscordEvent must have Type")
	}
	if event.GuildID == 0 {
		t.Error("DiscordEvent must have non-zero GuildID")
	}
}

// TestDiscordClientPortContract verifies DiscordClient interface contract.
func TestDiscordClientPortContract(t *testing.T) {
	var _ DiscordClient = (*mockDiscordClient)(nil)
}

// TestLLMClientPortContract verifies LLMClient interface contract.
func TestLLMClientPortContract(t *testing.T) {
	var _ LLMClient = (*mockLLMClient)(nil)
}

// TestEmbeddingClientPortContract verifies EmbeddingClient interface contract.
func TestEmbeddingClientPortContract(t *testing.T) {
	var _ EmbeddingClient = (*mockEmbeddingClient)(nil)
}

// TestImageClientPortContract verifies ImageClient interface contract.
func TestImageClientPortContract(t *testing.T) {
	var _ ImageClient = (*mockImageClient)(nil)
}

// TestStoragePortContract verifies StoragePort interface contract.
func TestStoragePortContract(t *testing.T) {
	var _ StoragePort = (*mockStoragePort)(nil)
}

// TestToolExecutorPortContract verifies ToolExecutor interface contract.
func TestToolExecutorPortContract(t *testing.T) {
	var _ ToolExecutor = (*mockToolExecutor)(nil)
}

// TestRetrievalPortContract verifies RetrievalPort interface contract.
func TestRetrievalPortContract(t *testing.T) {
	var _ RetrievalPort = (*mockRetrievalPort)(nil)
}

// Mock implementations for interface contract testing
type mockDiscordClient struct{}

func (m *mockDiscordClient) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	return nil
}

func (m *mockDiscordClient) GetMessage(ctx context.Context, guildID, channelID, messageID int64) (*DiscordMessage, error) {
	return nil, nil
}

func (m *mockDiscordClient) GetGuild(ctx context.Context, guildID int64) (*Guild, error) {
	return nil, nil
}

type mockLLMClient struct{}

func (m *mockLLMClient) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	return "", nil
}

func (m *mockLLMClient) CallTool(ctx context.Context, guildID int64, toolName string, arguments map[string]interface{}) (string, error) {
	return "", nil
}

type mockEmbeddingClient struct{}

func (m *mockEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
}

type mockImageClient struct{}

func (m *mockImageClient) Generate(ctx context.Context, prompt string) (string, error) {
	return "", nil
}

type mockStoragePort struct{}

func (m *mockStoragePort) SaveMemory(ctx context.Context, guildID int64, content string, embedding []float32) error {
	return nil
}

func (m *mockStoragePort) GetMemories(ctx context.Context, guildID int64, limit int) ([]MemoryRecord, error) {
	return nil, nil
}

func (m *mockStoragePort) SaveToolResult(ctx context.Context, result *ToolResult) error {
	return nil
}

func (m *mockStoragePort) GetToolResult(ctx context.Context, toolRequestID string) (*ToolResult, error) {
	return nil, nil
}

func (m *mockStoragePort) SaveLoreCitation(ctx context.Context, citation *LoreCitation) error {
	return nil
}

func (m *mockStoragePort) GetLoreCitations(ctx context.Context, guildID int64, limit int) ([]LoreCitation, error) {
	return nil, nil
}

func (m *mockStoragePort) SaveGuildConfig(ctx context.Context, config *GuildConfig) error {
	return nil
}

func (m *mockStoragePort) GetGuildConfig(ctx context.Context, guildID int64, key string) (*GuildConfig, error) {
	return nil, nil
}

func (m *mockStoragePort) AddExceptionChannel(ctx context.Context, guildID, channelID int64) error {
	return nil
}

func (m *mockStoragePort) IsExceptionChannel(ctx context.Context, guildID, channelID int64) (bool, error) {
	return false, nil
}

type mockToolExecutor struct{}

func (m *mockToolExecutor) Execute(ctx context.Context, request *ToolRequest) (*ToolResult, error) {
	return nil, nil
}

type mockRetrievalPort struct{}

func (m *mockRetrievalPort) IndexLore(ctx context.Context, guildID int64, content string, source string) error {
	return nil
}

func (m *mockRetrievalPort) RetrieveLore(ctx context.Context, guildID int64, query string, limit int) ([]LoreCitation, error) {
	return nil, nil
}
