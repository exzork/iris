package router

import (
	"context"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

// MockExceptionChannelRepo is a test double for exception channel queries.
type MockExceptionChannelRepo struct {
	exceptionChannels map[int64]map[int64]bool
}

func NewMockExceptionChannelRepo() *MockExceptionChannelRepo {
	return &MockExceptionChannelRepo{
		exceptionChannels: make(map[int64]map[int64]bool),
	}
}

func (m *MockExceptionChannelRepo) AddException(guildID, channelID int64) {
	if m.exceptionChannels[guildID] == nil {
		m.exceptionChannels[guildID] = make(map[int64]bool)
	}
	m.exceptionChannels[guildID][channelID] = true
}

func (m *MockExceptionChannelRepo) IsException(ctx context.Context, guildID int64, channelID int64) (bool, error) {
	return m.exceptionChannels[guildID][channelID], nil
}

// MockAllowedChannelRepo is a test double for allowed channel queries.
type MockAllowedChannelRepo struct {
	allowedChannels map[int64]map[int64]bool
}

func NewMockAllowedChannelRepo() *MockAllowedChannelRepo {
	return &MockAllowedChannelRepo{
		allowedChannels: make(map[int64]map[int64]bool),
	}
}

func (m *MockAllowedChannelRepo) Add(ctx context.Context, guildID int64, channelID int64) error {
	if m.allowedChannels[guildID] == nil {
		m.allowedChannels[guildID] = make(map[int64]bool)
	}
	m.allowedChannels[guildID][channelID] = true
	return nil
}

func (m *MockAllowedChannelRepo) Remove(ctx context.Context, guildID int64, channelID int64) error {
	delete(m.allowedChannels[guildID], channelID)
	return nil
}

func (m *MockAllowedChannelRepo) IsAllowed(ctx context.Context, guildID int64, channelID int64) (bool, error) {
	return m.allowedChannels[guildID][channelID], nil
}

func (m *MockAllowedChannelRepo) HasAny(ctx context.Context, guildID int64) (bool, error) {
	return len(m.allowedChannels[guildID]) > 0, nil
}

func (m *MockAllowedChannelRepo) ListByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	var result []int64
	for channelID := range m.allowedChannels[guildID] {
		result = append(result, channelID)
	}
	return result, nil
}

// MockChannelConversationRepo is a test double for channel conversation queries.
type MockChannelConversationRepo struct {
	activeChannels map[int64]map[int64]bool
}

func NewMockChannelConversationRepo() *MockChannelConversationRepo {
	return &MockChannelConversationRepo{
		activeChannels: make(map[int64]map[int64]bool),
	}
}

func (m *MockChannelConversationRepo) SetActive(guildID, channelID int64, active bool) {
	if m.activeChannels[guildID] == nil {
		m.activeChannels[guildID] = make(map[int64]bool)
	}
	m.activeChannels[guildID][channelID] = active
}

func (m *MockChannelConversationRepo) Refresh(ctx context.Context, guildID int64, channelID int64, now time.Time, ttl time.Duration) error {
	m.SetActive(guildID, channelID, true)
	return nil
}

func (m *MockChannelConversationRepo) Active(ctx context.Context, guildID int64, channelID int64, now time.Time) (bool, error) {
	return m.activeChannels[guildID][channelID], nil
}

func (m *MockChannelConversationRepo) Clear(ctx context.Context, guildID int64, channelID int64) error {
	if m.activeChannels[guildID] != nil {
		delete(m.activeChannels[guildID], channelID)
	}
	return nil
}

// TestDecideMention tests that mention trigger results in respond decision.
func TestDecideMention(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouter(mockRepo)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.Should {
		t.Errorf("expected Should=true for mention, got false")
	}

	if decision.Reason != ReasonMention {
		t.Errorf("expected reason %q, got %q", ReasonMention, decision.Reason)
	}
}

// TestDecideReply tests that reply-to-bot trigger results in respond decision.
func TestDecideReply(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouter(mockRepo)

	event := &domain.DiscordEvent{
		Type:      "message_reply",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "reply to bot",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.Should {
		t.Errorf("expected Should=true for reply, got false")
	}

	if decision.Reason != ReasonReply {
		t.Errorf("expected reason %q, got %q", ReasonReply, decision.Reason)
	}
}

// TestDecideNameMention tests that iris name mention results in respond decision.
func TestDecideNameMention(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouter(mockRepo)

	event := &domain.DiscordEvent{
		Type:      "message_content",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "hey iris, what's up?",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.Should {
		t.Errorf("expected Should=true for name mention, got false")
	}

	if decision.Reason != ReasonNameMention {
		t.Errorf("expected reason %q, got %q", ReasonNameMention, decision.Reason)
	}
}

// TestDecideExceptionChannel tests that exception channels suppress responses.
func TestDecideExceptionChannel(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	mockRepo.AddException(123, 456) // Add channel 456 as exception for guild 123

	router := NewTriggerRouter(mockRepo)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456, // Exception channel
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should {
		t.Errorf("expected Should=false for exception channel, got true")
	}

	if decision.Reason != ReasonExceptionChannel {
		t.Errorf("expected reason %q, got %q", ReasonExceptionChannel, decision.Reason)
	}
}

// TestDecideNoTrigger tests that unknown event type results in ignore decision.
func TestDecideNoTrigger(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouter(mockRepo)

	event := &domain.DiscordEvent{
		Type:      "message_casual",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "random message",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should {
		t.Errorf("expected Should=false for no trigger, got true")
	}

	if decision.Reason != ReasonNoTrigger {
		t.Errorf("expected reason %q, got %q", ReasonNoTrigger, decision.Reason)
	}
}

// TestDecideBotMessage tests that bot messages are ignored.
func TestDecideBotMessage(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouter(mockRepo)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    999, // Bot's own ID
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should {
		t.Errorf("expected Should=false for bot message, got true")
	}

	if decision.Reason != ReasonBotMessage {
		t.Errorf("expected reason %q, got %q", ReasonBotMessage, decision.Reason)
	}
}

// TestDecidePriorityMentionOverReply tests that mention has priority over reply.
func TestDecidePriorityMentionOverReply(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouter(mockRepo)

	event := &domain.DiscordEvent{
		Type:      "message_mention", // Mention takes priority
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> reply to bot",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Reason != ReasonMention {
		t.Errorf("expected reason %q (mention priority), got %q", ReasonMention, decision.Reason)
	}
}

// TestDecideExceptionChannelPriority tests that exception channel suppresses even high-priority triggers.
func TestDecideExceptionChannelPriority(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	mockRepo.AddException(123, 456)

	router := NewTriggerRouter(mockRepo)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should {
		t.Errorf("expected Should=false (exception channel suppresses mention), got true")
	}

	if decision.Reason != ReasonExceptionChannel {
		t.Errorf("expected reason %q, got %q", ReasonExceptionChannel, decision.Reason)
	}
}

// TestDecideFallbackModeEmptyAllowList tests that when no allowed channels exist (HasAny=false),
// the router falls back to exception-channel behavior.
func TestDecideFallbackModeEmptyAllowList(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	exceptionRepo.AddException(123, 789) // Channel 789 is exception
	allowedRepo := NewMockAllowedChannelRepo()
	// allowedRepo has no entries for guild 123, so HasAny returns false

	router := NewTriggerRouterWithAllowList(exceptionRepo, allowedRepo)

	// Test 1: Mention in non-exception channel should respond (fallback mode)
	event1 := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456, // Not in exception list
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision1, err := router.Decide(context.Background(), event1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision1.Should || decision1.Reason != ReasonMention {
		t.Errorf("fallback mode: expected respond with mention, got Should=%v Reason=%q", decision1.Should, decision1.Reason)
	}

	// Test 2: Mention in exception channel should ignore (fallback mode respects exceptions)
	event2 := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 789, // In exception list
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision2, err := router.Decide(context.Background(), event2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision2.Should || decision2.Reason != ReasonExceptionChannel {
		t.Errorf("fallback mode: expected ignore with exception_channel reason, got Should=%v Reason=%q", decision2.Should, decision2.Reason)
	}
}

// TestDecideIncludeListModeAllowedChannel tests that when allowed channels exist (HasAny=true),
// the router enters include-list mode and only responds in allowed channels.
func TestDecideIncludeListModeAllowedChannel(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	exceptionRepo.AddException(123, 789) // Exception list exists but should be ignored in include-list mode
	allowedRepo := NewMockAllowedChannelRepo()
	allowedRepo.Add(context.Background(), 123, 456) // Only channel 456 is allowed

	router := NewTriggerRouterWithAllowList(exceptionRepo, allowedRepo)

	// Test 1: Mention in allowed channel should respond
	event1 := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456, // In allowed list
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision1, err := router.Decide(context.Background(), event1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision1.Should || decision1.Reason != ReasonMention {
		t.Errorf("include-list mode: expected respond with mention in allowed channel, got Should=%v Reason=%q", decision1.Should, decision1.Reason)
	}

	// Test 2: Mention in non-allowed channel should ignore with channel_not_allowed reason
	event2 := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 999, // Not in allowed list
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision2, err := router.Decide(context.Background(), event2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision2.Should || decision2.Reason != ReasonChannelNotAllowed {
		t.Errorf("include-list mode: expected ignore with channel_not_allowed reason, got Should=%v Reason=%q", decision2.Should, decision2.Reason)
	}

	// Test 3: Mention in exception channel should still ignore with channel_not_allowed (not exception_channel)
	// because include-list mode ignores the exception list
	event3 := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 789, // In exception list but not in allowed list
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision3, err := router.Decide(context.Background(), event3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision3.Should || decision3.Reason != ReasonChannelNotAllowed {
		t.Errorf("include-list mode: expected ignore with channel_not_allowed (not exception_channel), got Should=%v Reason=%q", decision3.Should, decision3.Reason)
	}
}

// TestDecideIncludeListModeBotMessageStillIgnored tests that bot messages are ignored
// even in include-list mode (bot check runs before channel check).
func TestDecideIncludeListModeBotMessageStillIgnored(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()
	allowedRepo.Add(context.Background(), 123, 456)

	router := NewTriggerRouterWithAllowList(exceptionRepo, allowedRepo)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456, // Allowed channel
		UserID:    999, // Bot's own ID
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonBotMessage {
		t.Errorf("include-list mode: expected bot message ignored with bot_message reason, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}

// TestDecide_ActiveConversation_ElevatesCasualMessage tests that a casual message in an active conversation window is elevated to Respond.
func TestDecide_ActiveConversation_ElevatesCasualMessage(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()
	convRepo := NewMockChannelConversationRepo()
	convRepo.SetActive(123, 456, true)

	router := &TriggerRouter{
		exceptionChannelRepo: exceptionRepo,
		allowedChannelRepo:   allowedRepo,
		convRepo:             convRepo,
		botID:                999,
	}

	event := &domain.DiscordEvent{
		Type:      "message_casual",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "just a casual message",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.Should || decision.Reason != ReasonActiveConversation {
		t.Errorf("expected Respond with active_conversation, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}

// TestDecide_ActiveConversation_RespectsIncludeListGuard tests that include-list guard takes precedence over active conversation.
func TestDecide_ActiveConversation_RespectsIncludeListGuard(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()
	allowedRepo.Add(context.Background(), 123, 999) // Only channel 999 is allowed
	convRepo := NewMockChannelConversationRepo()
	convRepo.SetActive(123, 456, true) // Channel 456 is active but not allowed

	router := &TriggerRouter{
		exceptionChannelRepo: exceptionRepo,
		allowedChannelRepo:   allowedRepo,
		convRepo:             convRepo,
		botID:                999,
	}

	event := &domain.DiscordEvent{
		Type:      "message_casual",
		GuildID:   123,
		ChannelID: 456, // Not in allowed list
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "casual message in non-allowed channel",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonChannelNotAllowed {
		t.Errorf("expected Ignore with channel_not_allowed (guard precedence), got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}

// TestDecide_ActiveConversation_BotStillIgnored tests that bot messages are ignored even in active conversation.
func TestDecide_ActiveConversation_BotStillIgnored(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()
	convRepo := NewMockChannelConversationRepo()
	convRepo.SetActive(123, 456, true)

	router := &TriggerRouter{
		exceptionChannelRepo: exceptionRepo,
		allowedChannelRepo:   allowedRepo,
		convRepo:             convRepo,
		botID:                999,
	}

	event := &domain.DiscordEvent{
		Type:      "message_casual",
		GuildID:   123,
		ChannelID: 456,
		UserID:    999, // Bot's own ID
		Message: &domain.DiscordMessage{
			Content: "bot's own message",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonBotMessage {
		t.Errorf("expected Ignore with bot_message, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}

// TestDecide_ActiveConversation_MentionStillWins tests that mention trigger takes precedence over active conversation.
func TestDecide_ActiveConversation_MentionStillWins(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()
	convRepo := NewMockChannelConversationRepo()
	convRepo.SetActive(123, 456, true)

	router := &TriggerRouter{
		exceptionChannelRepo: exceptionRepo,
		allowedChannelRepo:   allowedRepo,
		convRepo:             convRepo,
		botID:                999,
	}

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.Should || decision.Reason != ReasonMention {
		t.Errorf("expected Respond with mention (precedence), got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}

// TestDecide_ActiveConversation_InactiveFallsThroughToNoTrigger tests that inactive conversation falls through to no trigger.
func TestDecide_ActiveConversation_InactiveFallsThroughToNoTrigger(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()
	convRepo := NewMockChannelConversationRepo()
	convRepo.SetActive(123, 456, false) // Conversation is inactive

	router := &TriggerRouter{
		exceptionChannelRepo: exceptionRepo,
		allowedChannelRepo:   allowedRepo,
		convRepo:             convRepo,
		botID:                999,
	}

	event := &domain.DiscordEvent{
		Type:      "message_casual",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "casual message in inactive channel",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonNoTrigger {
		t.Errorf("expected Ignore with no_trigger, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}

// TestDecide_ActiveConversation_NilQuerierIdenticalToCurrentRouter tests that nil convRepo behaves like the current router.
func TestDecide_ActiveConversation_NilQuerierIdenticalToCurrentRouter(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()

	router := &TriggerRouter{
		exceptionChannelRepo: exceptionRepo,
		allowedChannelRepo:   allowedRepo,
		convRepo:             nil, // Nil conversation repo
		botID:                999,
	}

	event := &domain.DiscordEvent{
		Type:      "message_casual",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "casual message with nil convRepo",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonNoTrigger {
		t.Errorf("expected Ignore with no_trigger (nil convRepo fallback), got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}

// TestDecide_CasualMessage_ElevatesWhenActive tests that casual messages (message_casual type) are elevated to Respond when conversation is active.
func TestDecide_CasualMessage_ElevatesWhenActive(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()
	convRepo := NewMockChannelConversationRepo()
	convRepo.SetActive(123, 456, true)

	router := &TriggerRouter{
		exceptionChannelRepo: exceptionRepo,
		allowedChannelRepo:   allowedRepo,
		convRepo:             convRepo,
		botID:                999,
	}

	event := &domain.DiscordEvent{
		Type:      "message_casual",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "just a casual message",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.Should || decision.Reason != ReasonActiveConversation {
		t.Errorf("expected Respond with active_conversation for casual message, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}

// TestDecide_CasualMessage_IgnoresWhenInactive tests that casual messages are ignored when conversation is not active.
func TestDecide_CasualMessage_IgnoresWhenInactive(t *testing.T) {
	exceptionRepo := NewMockExceptionChannelRepo()
	allowedRepo := NewMockAllowedChannelRepo()
	convRepo := NewMockChannelConversationRepo()
	convRepo.SetActive(123, 456, false)

	router := &TriggerRouter{
		exceptionChannelRepo: exceptionRepo,
		allowedChannelRepo:   allowedRepo,
		convRepo:             convRepo,
		botID:                999,
	}

	event := &domain.DiscordEvent{
		Type:      "message_casual",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "just a casual message",
		},
	}

	decision, err := router.Decide(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonNoTrigger {
		t.Errorf("expected Ignore with no_trigger for inactive casual message, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}
}
