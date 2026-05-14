package orchestrator

import (
	"context"
	"sync"

	"github.com/eko/iris-bot/internal/domain"
)

// FakeLoreAnchorResolver implements LoreAnchorResolver for testing.
type FakeLoreAnchorResolver struct {
	anchors map[int64]*domain.LoreThreadAnchor
	mu      sync.RWMutex
}

func NewFakeLoreAnchorResolver() *FakeLoreAnchorResolver {
	return &FakeLoreAnchorResolver{
		anchors: make(map[int64]*domain.LoreThreadAnchor),
	}
}

func (f *FakeLoreAnchorResolver) GetByThread(ctx context.Context, guildID int64, threadID int64) (*domain.LoreThreadAnchor, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if a, ok := f.anchors[threadID]; ok {
		return a, nil
	}
	return nil, nil
}

func (f *FakeLoreAnchorResolver) AddAnchor(anchor *domain.LoreThreadAnchor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.anchors[anchor.ThreadID] = anchor
}

// FakeChannelNameResolver implements ChannelNameResolver for testing.
type FakeChannelNameResolver struct {
	names map[int64][2]string
	mu    sync.RWMutex
}

func NewFakeChannelNameResolver() *FakeChannelNameResolver {
	return &FakeChannelNameResolver{
		names: make(map[int64][2]string),
	}
}

func (f *FakeChannelNameResolver) Resolve(ctx context.Context, threadID int64) (channelName, threadName string, ok bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if names, exists := f.names[threadID]; exists {
		return names[0], names[1], true
	}
	return "", "", false
}

func (f *FakeChannelNameResolver) SetNames(threadID int64, channelName, threadName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.names[threadID] = [2]string{channelName, threadName}
}

// FakeAllowedChannelLister implements AllowedChannelLister for testing.
type FakeAllowedChannelLister struct {
	allowed map[int64][]int64
	mu      sync.RWMutex
}

func NewFakeAllowedChannelLister() *FakeAllowedChannelLister {
	return &FakeAllowedChannelLister{
		allowed: make(map[int64][]int64),
	}
}

func (f *FakeAllowedChannelLister) ListByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if channels, ok := f.allowed[guildID]; ok {
		return channels, nil
	}
	return nil, nil
}

func (f *FakeAllowedChannelLister) SetAllowed(guildID int64, channelIDs []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowed[guildID] = channelIDs
}

// FakeEpisodeArchiver implements EpisodeArchiver for testing.
type FakeEpisodeArchiver struct {
	archived []string
	mu       sync.RWMutex
}

func NewFakeEpisodeArchiver() *FakeEpisodeArchiver {
	return &FakeEpisodeArchiver{
		archived: []string{},
	}
}

func (f *FakeEpisodeArchiver) Archive(ctx context.Context, guildID int64, messages []*domain.ChannelMessage, taggedLines []string, resolver ChannelNameResolver) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, line := range taggedLines {
		f.archived = append(f.archived, line)
	}
	return nil
}

func (f *FakeEpisodeArchiver) ArchivedCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.archived)
}

func (f *FakeEpisodeArchiver) GetArchived(index int) string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if index < len(f.archived) {
		return f.archived[index]
	}
	return ""
}

// FakeCompactor implements Compactor for testing.
type FakeCompactor struct {
	compactFn func(ctx context.Context, guildID int64, lines []string) ([]string, error)
}

func NewFakeCompactor(compactFn func(ctx context.Context, guildID int64, lines []string) ([]string, error)) *FakeCompactor {
	return &FakeCompactor{compactFn: compactFn}
}

func (f *FakeCompactor) Compact(ctx context.Context, guildID int64, lines []string) ([]string, error) {
	if f.compactFn != nil {
		return f.compactFn(ctx, guildID, lines)
	}
	return lines, nil
}
