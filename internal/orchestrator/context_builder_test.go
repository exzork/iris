package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

// MockContextStore implements ContextStore for testing.
type MockContextStore struct {
	messages map[int64][]*domain.ChannelMessage
	byID     map[int64]*domain.ChannelMessage
}

func NewMockContextStore() *MockContextStore {
	return &MockContextStore{
		messages: make(map[int64][]*domain.ChannelMessage),
		byID:     make(map[int64]*domain.ChannelMessage),
	}
}

func (m *MockContextStore) ListRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*domain.ChannelMessage, error) {
	key := guildID*1e9 + channelID
	msgs := m.messages[key]
	if len(msgs) > limit {
		return msgs[len(msgs)-limit:], nil
	}
	return msgs, nil
}

func (m *MockContextStore) GetByID(ctx context.Context, guildID, messageID int64) (*domain.ChannelMessage, error) {
	return m.byID[messageID], nil
}

func (m *MockContextStore) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	return nil, nil
}

func (m *MockContextStore) addMessage(guildID, channelID, messageID, userID int64, authorName, content string, isBot bool, replyToID *int64) {
	key := guildID*1e9 + channelID
	msg := &domain.ChannelMessage{
		GuildID:          guildID,
		ChannelID:        channelID,
		MessageID:        messageID,
		UserID:           userID,
		AuthorName:       &authorName,
		Content:          content,
		IsBot:            isBot,
		ReplyToMessageID: replyToID,
		CreatedAt:        time.Now().Add(-time.Duration(messageID) * time.Second),
	}
	m.messages[key] = append(m.messages[key], msg)
	m.byID[messageID] = msg
}

func TestContextBuilderBasicMention(t *testing.T) {
	store := NewMockContextStore()
	store.addMessage(1, 100, 1, 10, "alice", "hello", false, nil)
	store.addMessage(1, 100, 2, 11, "bob", "hi there", false, nil)
	store.addMessage(1, 100, 3, 12, "charlie", "how are you", false, nil)

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	})

	currentEvent := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    12,
		AuthorName: func() *string {
			s := "charlie"
			return &s
		}(),
		Message: &domain.DiscordMessage{
			ID:      4,
			Content: "what's up?",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.Build(context.Background(), currentEvent, store, "You are a helpful bot.")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Expect: system, 3 prior messages, current message = 5 total
	if len(messages) < 4 {
		t.Errorf("expected at least 4 messages (system + 2 prior + current), got %d", len(messages))
	}

	// First should be system
	if messages[0]["role"] != "system" {
		t.Errorf("first message role should be 'system', got %q", messages[0]["role"])
	}
	if messages[0]["content"] != "You are a helpful bot." {
		t.Errorf("system prompt mismatch")
	}

	// Verify no memory or behavior blocks have system role
	for i, msg := range messages {
		if msg["role"] == "system" && i > 0 {
			t.Errorf("message %d has system role but should be user (only first message is system)", i)
		}
	}

	// Last should be current user message
	if messages[len(messages)-1]["role"] != "user" {
		t.Errorf("last message role should be 'user', got %q", messages[len(messages)-1]["role"])
	}
	if !contains(messages[len(messages)-1]["content"], "what's up?") {
		t.Errorf("last message should contain current content")
	}
}

func TestContextBuilderReplyChain(t *testing.T) {
	store := NewMockContextStore()
	// Message 1: alice
	store.addMessage(1, 100, 1, 10, "alice", "first message", false, nil)
	// Message 2: bob replies to 1
	replyTo1 := int64(1)
	store.addMessage(1, 100, 2, 11, "bob", "reply to alice", false, &replyTo1)
	// Message 3: charlie replies to 2
	replyTo2 := int64(2)
	store.addMessage(1, 100, 3, 12, "charlie", "reply to bob", false, &replyTo2)
	// Message 4: dave replies to 3
	replyTo3 := int64(3)
	store.addMessage(1, 100, 4, 13, "dave", "reply to charlie", false, &replyTo3)
	// Message 5: eve replies to 4 (depth 4, should be capped at 3)
	replyTo4 := int64(4)
	store.addMessage(1, 100, 5, 14, "eve", "reply to dave", false, &replyTo4)

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	})

	currentEvent := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    15,
		AuthorName: func() *string {
			s := "frank"
			return &s
		}(),
		Message: &domain.DiscordMessage{
			ID:      6,
			Content: "final reply",
		},
		ReplyToMessageID: &replyTo4,
		CreatedAt:        time.Now(),
	}

	messages, err := builder.Build(context.Background(), currentEvent, store, "You are a helpful bot.")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should include system, prior messages, reply chain (up to 3 ancestors), and current
	// Verify reply chain is included
	foundReplyContent := false
	for _, msg := range messages {
		if contains(msg["content"], "reply to") {
			foundReplyContent = true
			break
		}
	}
	if !foundReplyContent {
		t.Errorf("expected reply chain content in messages")
	}

	// Last message should be current
	if !contains(messages[len(messages)-1]["content"], "final reply") {
		t.Errorf("last message should be current message")
	}
}

func TestContextBuilderMinContext(t *testing.T) {
	store := NewMockContextStore()
	// Add only 5 messages
	for i := 1; i <= 5; i++ {
		store.addMessage(1, 100, int64(i), int64(10+i), "user"+string(rune(i)), "msg "+string(rune(i)), false, nil)
	}

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	})

	currentEvent := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    20,
		AuthorName: func() *string {
			s := "user"
			return &s
		}(),
		Message: &domain.DiscordMessage{
			ID:      6,
			Content: "query",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.Build(context.Background(), currentEvent, store, "System prompt")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Should include system + all 5 prior + current = 7 messages
	// MinContext is a floor, not a requirement to inflate
	if len(messages) < 3 {
		t.Errorf("expected at least 3 messages (system + prior + current), got %d", len(messages))
	}
}

func TestContextBuilderTruncation(t *testing.T) {
	store := NewMockContextStore()
	longContent := strings.Repeat("a", 601)
	store.addMessage(1, 100, 1, 10, "alice", longContent, false, nil)

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	})

	currentEvent := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    11,
		AuthorName: func() *string {
			s := "bob"
			return &s
		}(),
		Message: &domain.DiscordMessage{
			ID:      2,
			Content: "short",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.Build(context.Background(), currentEvent, store, "System")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Find the truncated message
	for _, msg := range messages {
		if contains(msg["content"], "alice") {
			// Should have truncation marker
			if !contains(msg["content"], "…") {
				t.Errorf("expected truncation marker in long message")
			}
			break
		}
	}
}

func TestContextBuilderBotMessages(t *testing.T) {
	store := NewMockContextStore()
	store.addMessage(1, 100, 1, 10, "alice", "hello", false, nil)
	store.addMessage(1, 100, 2, 999, "iris", "hi alice", true, nil)
	store.addMessage(1, 100, 3, 11, "bob", "how are you", false, nil)

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	})

	currentEvent := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    12,
		AuthorName: func() *string {
			s := "charlie"
			return &s
		}(),
		Message: &domain.DiscordMessage{
			ID:      4,
			Content: "what's up?",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.Build(context.Background(), currentEvent, store, "System")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Bot message should have role "assistant"
	foundBotAssistant := false
	for _, msg := range messages {
		if msg["role"] == "assistant" && contains(msg["content"], "iris") {
			foundBotAssistant = true
			break
		}
	}
	if !foundBotAssistant {
		t.Errorf("expected bot message with role 'assistant'")
	}
}

func TestContextBuilderLineFormat(t *testing.T) {
	store := NewMockContextStore()
	store.addMessage(1, 100, 1, 10, "alice", "test message", false, nil)

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	})

	currentEvent := &domain.DiscordEvent{
		GuildID:   1,
		ChannelID: 100,
		UserID:    11,
		AuthorName: func() *string {
			s := "bob"
			return &s
		}(),
		Message: &domain.DiscordMessage{
			ID:      2,
			Content: "reply",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.Build(context.Background(), currentEvent, store, "System")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Line format contract:
	// - prior-context messages: "<username> (user id: N): <content>" or plain content if no author
	// - current user message: same identity-prefixed format so the LLM can
	//   disambiguate the caller from other speakers in the same channel.
	// - no channel IDs, no timestamps, no "user:N" fallbacks leak into any message
	// This protects against the LLM mimicking internal metadata in its reply.
	for i, msg := range messages {
		if msg["role"] == "system" {
			continue
		}
		content := msg["content"]
		if contains(content, "user:") {
			t.Errorf("message %d leaked raw user-ID fallback: %q", i, content)
		}
		if contains(content, "[100") || contains(content, "· ") {
			t.Errorf("message %d leaked bracketed channel/timestamp metadata: %q", i, content)
		}
		if i == len(messages)-1 {
			want := "bob (user id: 11): reply"
			if content != want {
				t.Errorf("current user message should be %q, got %q", want, content)
			}
			continue
		}
		if !contains(content, "alice") && !contains(content, "bob") {
			t.Errorf("prior message %d missing author label: %q", i, content)
		}
	}
}

func TestContextBuilderWithLoreAnchor(t *testing.T) {
	store := NewMockContextStore()
	store.addMessage(1, 100, 1, 10, "alice", "lore discussion 1", false, nil)
	store.addMessage(1, 100, 2, 11, "bob", "lore discussion 2", false, nil)

	now := time.Now().UTC()
	anchor := &domain.LoreThreadAnchor{
		ID:               1,
		GuildID:          1,
		ChannelID:        100,
		ThreadID:         200,
		SummaryMessageID: nil,
		SummaryText:      strPtr("This is the lore summary"),
		Title:            strPtr("Lore Thread"),
		SourceSessionID:  nil,
		CreatedAt:        now,
	}

	mockResolver := &mockLoreAnchorResolver{anchor: anchor}
	mockNameResolver := &mockChannelNameResolver{
		channels: map[int64][2]string{
			200: {"lore-channel", "lore-thread"},
		},
	}
	mockAllowed := &mockAllowedChannelLister{allowed: []int64{100}}

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	}).
		WithLoreAnchorResolver(mockResolver).
		WithChannelNames(mockNameResolver).
		WithAllowedChannels(mockAllowed)

	currentEvent := &domain.DiscordEvent{
		GuildID:    1,
		ChannelID:  100,
		ThreadID:   200,
		UserID:     12,
		AuthorName: strPtr("charlie"),
		Message: &domain.DiscordMessage{
			ID:      3,
			Content: "follow-up question",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.BuildWithCrossChannel(context.Background(), currentEvent, store, "System", nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	foundAnchor := false
	for _, msg := range messages {
		if contains(msg["content"], "This is the lore summary") {
			foundAnchor = true
			break
		}
	}
	if !foundAnchor {
		t.Errorf("expected anchor summary in context")
	}
}

func TestContextBuilderWithLoreAnchor_ParentChannelNotAllowed(t *testing.T) {
	store := NewMockContextStore()
	store.addMessage(1, 100, 1, 10, "alice", "message", false, nil)

	now := time.Now().UTC()
	anchor := &domain.LoreThreadAnchor{
		ID:               1,
		GuildID:          1,
		ChannelID:        100,
		ThreadID:         200,
		SummaryMessageID: nil,
		SummaryText:      strPtr("This is the lore summary"),
		Title:            nil,
		SourceSessionID:  nil,
		CreatedAt:        now,
	}

	mockResolver := &mockLoreAnchorResolver{anchor: anchor}
	mockAllowed := &mockAllowedChannelLister{allowed: []int64{999}}

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	}).
		WithLoreAnchorResolver(mockResolver).
		WithAllowedChannels(mockAllowed)

	currentEvent := &domain.DiscordEvent{
		GuildID:    1,
		ChannelID:  100,
		ThreadID:   200,
		UserID:     12,
		AuthorName: strPtr("charlie"),
		Message: &domain.DiscordMessage{
			ID:      2,
			Content: "question",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.BuildWithCrossChannel(context.Background(), currentEvent, store, "System", nil)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	foundAnchor := false
	for _, msg := range messages {
		if contains(msg["content"], "This is the lore summary") {
			foundAnchor = true
			break
		}
	}
	if foundAnchor {
		t.Errorf("expected anchor to NOT be injected when parent channel not allowed")
	}
}

func TestContextBuilderWithoutLoreAnchor_NonThreadMessage(t *testing.T) {
	store := NewMockContextStore()
	store.addMessage(1, 100, 1, 10, "alice", "message", false, nil)

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	})

	currentEvent := &domain.DiscordEvent{
		GuildID:    1,
		ChannelID:  100,
		ThreadID:   0,
		UserID:     12,
		AuthorName: strPtr("charlie"),
		Message: &domain.DiscordMessage{
			ID:      2,
			Content: "question",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.Build(context.Background(), currentEvent, store, "System")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(messages) < 2 {
		t.Errorf("expected at least system + current message")
	}
}

func TestFormatUserLabel(t *testing.T) {
	cases := []struct {
		name       string
		authorName *string
		userID     int64
		isBot      bool
		want       string
	}{
		{"username and id", strPtr("eko"), 111, false, "eko (user id: 111)"},
		{"id only fallback", nil, 222, false, "user id: 222"},
		{"empty username string falls back to id", strPtr(""), 333, false, "user id: 333"},
		{"bot keeps name without claimed id", strPtr("iris"), 444, true, "iris"},
		{"bot without name returns empty", nil, 444, true, ""},
		{"missing user id with name returns empty-id", strPtr("anon"), 0, false, "anon"},
		{"missing everything returns empty", nil, 0, false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatUserLabel(tc.authorName, tc.userID, tc.isBot)
			if got != tc.want {
				t.Errorf("formatUserLabel(%v, %d, %v) = %q, want %q", tc.authorName, tc.userID, tc.isBot, got, tc.want)
			}
		})
	}
}

func TestContextBuilder_BusyChannel_DistinguishesUsersByID(t *testing.T) {
	store := NewMockContextStore()
	store.addMessage(1, 100, 1, 111, "eko", "first message", false, nil)
	store.addMessage(1, 100, 2, 222, "mika", "different speaker", false, nil)

	builder := NewContextBuilder(ContextBuilderConfig{
		MinContext:        10,
		CurrentChannelMax: 20,
		ReplyDepthLimit:   3,
		PerMessageCharCap: 500,
	})

	currentEvent := &domain.DiscordEvent{
		GuildID:    1,
		ChannelID:  100,
		UserID:     111,
		AuthorName: strPtr("eko"),
		Message: &domain.DiscordMessage{
			ID:      3,
			Content: "current ping",
		},
		CreatedAt: time.Now(),
	}

	messages, err := builder.Build(context.Background(), currentEvent, store, "sys")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	combined := ""
	for _, m := range messages {
		combined += "\n" + m["content"]
	}

	for _, want := range []string{
		"eko (user id: 111): first message",
		"mika (user id: 222): different speaker",
		"eko (user id: 111): current ping",
	} {
		if !contains(combined, want) {
			t.Errorf("missing %q in rendered context:\n%s", want, combined)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func strPtr(s string) *string {
	return &s
}
