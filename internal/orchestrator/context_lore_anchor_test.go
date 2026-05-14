package orchestrator

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

type mockLoreAnchorResolver struct {
	anchor *domain.LoreThreadAnchor
	err    error
}

func (m *mockLoreAnchorResolver) GetByThread(ctx context.Context, guildID int64, threadID int64) (*domain.LoreThreadAnchor, error) {
	return m.anchor, m.err
}

type mockChannelNameResolver struct {
	channels map[int64][2]string // threadID -> [channelName, threadName]
}

func (m *mockChannelNameResolver) Resolve(ctx context.Context, channelID int64) (string, string, bool) {
	if names, ok := m.channels[channelID]; ok {
		return names[0], names[1], true
	}
	return "", "", false
}

type mockAllowedChannelLister struct {
	allowed []int64
	err     error
}

func (m *mockAllowedChannelLister) ListByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	return m.allowed, m.err
}

func TestBuildLoreAnchorLines_WithValidAnchor(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	anchor := &domain.LoreThreadAnchor{
		ID:               1,
		GuildID:          123,
		ChannelID:        456,
		ThreadID:         789,
		SummaryMessageID: nil,
		SummaryText:      strPtr("This is a lore summary"),
		Title:            strPtr("Lore Thread Title"),
		SourceSessionID:  nil,
		CreatedAt:        now,
	}

	resolver := &mockChannelNameResolver{
		channels: map[int64][2]string{
			789: {"lore-channel", "thread-name"},
		},
	}

	lines, err := buildLoreAnchorLines(ctx, resolver, anchor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	line := lines[0]
	if !strings.HasPrefix(line, "[ANCHOR] ") {
		t.Errorf("expected [ANCHOR] prefix in line: %s", line)
	}
	if !contains(line, "lore-channel") {
		t.Errorf("expected channel name in line: %s", line)
	}
	if !contains(line, "thread-name") {
		t.Errorf("expected thread name in line: %s", line)
	}
	if !contains(line, "This is a lore summary") {
		t.Errorf("expected summary text in line: %s", line)
	}
	if !contains(line, "|0|") {
		t.Errorf("expected user ID 0 in line: %s", line)
	}
}

func TestBuildLoreAnchorLines_WithNilAnchor(t *testing.T) {
	ctx := context.Background()
	lines, err := buildLoreAnchorLines(ctx, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines for nil anchor, got %d", len(lines))
	}
}

func TestBuildLoreAnchorLines_WithNilSummaryText(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	anchor := &domain.LoreThreadAnchor{
		ID:               1,
		GuildID:          123,
		ChannelID:        456,
		ThreadID:         789,
		SummaryMessageID: nil,
		SummaryText:      nil,
		Title:            strPtr("Lore Thread Title"),
		SourceSessionID:  nil,
		CreatedAt:        now,
	}

	lines, err := buildLoreAnchorLines(ctx, nil, anchor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	line := lines[0]
	if !contains(line, "|0|") {
		t.Errorf("expected user ID 0 in line: %s", line)
	}
}

func TestBuildLoreAnchorLines_TruncatesLongSummary(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	longText := string(make([]byte, 600))
	for i := range longText {
		longText = longText[:i] + "a" + longText[i+1:]
	}

	anchor := &domain.LoreThreadAnchor{
		ID:               1,
		GuildID:          123,
		ChannelID:        456,
		ThreadID:         789,
		SummaryMessageID: nil,
		SummaryText:      &longText,
		Title:            nil,
		SourceSessionID:  nil,
		CreatedAt:        now,
	}

	lines, err := buildLoreAnchorLines(ctx, nil, anchor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
}

func TestIsThreadAllowed_ParentChannelAllowed(t *testing.T) {
	ctx := context.Background()

	anchor := &domain.LoreThreadAnchor{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		ThreadID:  789,
	}

	lister := &mockAllowedChannelLister{
		allowed: []int64{456, 789},
	}

	allowed, err := isThreadAllowed(ctx, lister, anchor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("expected thread to be allowed")
	}
}

func TestIsThreadAllowed_ParentChannelNotAllowed(t *testing.T) {
	ctx := context.Background()

	anchor := &domain.LoreThreadAnchor{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		ThreadID:  789,
	}

	lister := &mockAllowedChannelLister{
		allowed: []int64{999, 888},
	}

	allowed, err := isThreadAllowed(ctx, lister, anchor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("expected thread to NOT be allowed")
	}
}

func TestIsThreadAllowed_NoAllowedChannelsConfigured(t *testing.T) {
	ctx := context.Background()

	anchor := &domain.LoreThreadAnchor{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		ThreadID:  789,
	}

	lister := &mockAllowedChannelLister{
		allowed: []int64{},
	}

	allowed, err := isThreadAllowed(ctx, lister, anchor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("expected thread to be allowed when no channels configured")
	}
}

func TestIsThreadAllowed_NilAnchor(t *testing.T) {
	ctx := context.Background()
	lister := &mockAllowedChannelLister{allowed: []int64{456}}

	allowed, err := isThreadAllowed(ctx, lister, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("expected nil anchor to not be allowed")
	}
}

func TestIsThreadAllowed_NilLister(t *testing.T) {
	ctx := context.Background()

	anchor := &domain.LoreThreadAnchor{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		ThreadID:  789,
	}

	allowed, err := isThreadAllowed(ctx, nil, anchor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Errorf("expected nil lister to not be allowed")
	}
}

func TestIsThreadAllowed_ListError(t *testing.T) {
	ctx := context.Background()

	anchor := &domain.LoreThreadAnchor{
		ID:        1,
		GuildID:   123,
		ChannelID: 456,
		ThreadID:  789,
	}

	lister := &mockAllowedChannelLister{
		err: sql.ErrNoRows,
	}

	_, err := isThreadAllowed(ctx, lister, anchor)
	if err == nil {
		t.Errorf("expected error from lister")
	}
}
