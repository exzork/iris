package testutil

import (
	"context"
	"sync"

	"github.com/eko/iris-bot/internal/domain"
)

// FakeStoragePort implements domain.StoragePort for testing.
type FakeStoragePort struct {
	memories        []domain.MemoryRecord
	toolResults     map[string]*domain.ToolResult
	loreCitations   []domain.LoreCitation
	guildConfigs    map[string]*domain.GuildConfig
	exceptionChans  map[int64]map[int64]bool
	mu              sync.RWMutex
}

// NewFakeStoragePort creates a new fake storage port.
func NewFakeStoragePort() *FakeStoragePort {
	return &FakeStoragePort{
		memories:       []domain.MemoryRecord{},
		toolResults:    make(map[string]*domain.ToolResult),
		loreCitations:  []domain.LoreCitation{},
		guildConfigs:   make(map[string]*domain.GuildConfig),
		exceptionChans: make(map[int64]map[int64]bool),
	}
}

// SaveMemory saves a memory record.
func (f *FakeStoragePort) SaveMemory(ctx context.Context, guildID int64, content string, embedding []float32) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.memories = append(f.memories, domain.MemoryRecord{
		ID:        int64(len(f.memories) + 1),
		GuildID:   guildID,
		Content:   content,
		Embedding: embedding,
	})
	return nil
}

// GetMemories retrieves memories for a guild.
func (f *FakeStoragePort) GetMemories(ctx context.Context, guildID int64, limit int) ([]domain.MemoryRecord, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var result []domain.MemoryRecord
	for _, m := range f.memories {
		if m.GuildID == guildID {
			result = append(result, m)
		}
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// SaveToolResult saves a tool result.
func (f *FakeStoragePort) SaveToolResult(ctx context.Context, result *domain.ToolResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.toolResults[result.ID] = result
	return nil
}

// GetToolResult retrieves a tool result.
func (f *FakeStoragePort) GetToolResult(ctx context.Context, toolRequestID string) (*domain.ToolResult, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if result, exists := f.toolResults[toolRequestID]; exists {
		return result, nil
	}
	return nil, nil
}

// SaveLoreCitation saves a lore citation.
func (f *FakeStoragePort) SaveLoreCitation(ctx context.Context, citation *domain.LoreCitation) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.loreCitations = append(f.loreCitations, *citation)
	return nil
}

// GetLoreCitations retrieves lore citations for a guild.
func (f *FakeStoragePort) GetLoreCitations(ctx context.Context, guildID int64, limit int) ([]domain.LoreCitation, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	var result []domain.LoreCitation
	for _, c := range f.loreCitations {
		if c.GuildID == guildID {
			result = append(result, c)
		}
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// SaveGuildConfig saves a guild configuration.
func (f *FakeStoragePort) SaveGuildConfig(ctx context.Context, config *domain.GuildConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := configKey(config.GuildID, config.SettingKey)
	f.guildConfigs[key] = config
	return nil
}

// GetGuildConfig retrieves a guild configuration.
func (f *FakeStoragePort) GetGuildConfig(ctx context.Context, guildID int64, key string) (*domain.GuildConfig, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if config, exists := f.guildConfigs[configKey(guildID, key)]; exists {
		return config, nil
	}
	return nil, nil
}

// AddExceptionChannel adds an exception channel.
func (f *FakeStoragePort) AddExceptionChannel(ctx context.Context, guildID, channelID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, exists := f.exceptionChans[guildID]; !exists {
		f.exceptionChans[guildID] = make(map[int64]bool)
	}
	f.exceptionChans[guildID][channelID] = true
	return nil
}

// IsExceptionChannel checks if a channel is an exception channel.
func (f *FakeStoragePort) IsExceptionChannel(ctx context.Context, guildID, channelID int64) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if guildChans, exists := f.exceptionChans[guildID]; exists {
		return guildChans[channelID], nil
	}
	return false, nil
}

// GetAllMemories returns all stored memories for verification.
func (f *FakeStoragePort) GetAllMemories() []domain.MemoryRecord {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]domain.MemoryRecord{}, f.memories...)
}

// GetAllLoreCitations returns all stored lore citations for verification.
func (f *FakeStoragePort) GetAllLoreCitations() []domain.LoreCitation {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return append([]domain.LoreCitation{}, f.loreCitations...)
}

func configKey(guildID int64, key string) string {
	return key + ":" + string(rune(guildID))
}
