package router

import (
	"context"
	"testing"

	"github.com/eko/iris-bot/internal/domain"
)

func TestQAScenario_MentionTrigger(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouterWithBotID(mockRepo, 999)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> what's the lore about Encore?",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("QA Mention: unexpected error: %v", err)
	}

	if !decision.Should || decision.Reason != ReasonMention {
		t.Errorf("QA Mention: expected respond with mention reason, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}

	t.Logf("✓ QA Mention: Bot responds to direct mention with reason=%q", decision.Reason)
}

func TestQAScenario_ReplyTrigger(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouterWithBotID(mockRepo, 999)

	event := &domain.DiscordEvent{
		Type:      "message_reply",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "tell me more about that",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("QA Reply: unexpected error: %v", err)
	}

	if !decision.Should || decision.Reason != ReasonReply {
		t.Errorf("QA Reply: expected respond with reply reason, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}

	t.Logf("✓ QA Reply: Bot responds to reply-to-bot with reason=%q", decision.Reason)
}

func TestQAScenario_IrisNameMention(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouterWithBotID(mockRepo, 999)

	event := &domain.DiscordEvent{
		Type:      "message_content",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "hey iris, what do you think about this?",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("QA Iris Name: unexpected error: %v", err)
	}

	if !decision.Should || decision.Reason != ReasonNameMention {
		t.Errorf("QA Iris Name: expected respond with name_mention reason, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}

	t.Logf("✓ QA Iris Name: Bot responds to 'iris' name mention with reason=%q", decision.Reason)
}

func TestQAScenario_ExceptionChannelDenylist(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	mockRepo.AddException(123, 789)

	router := NewTriggerRouterWithBotID(mockRepo, 999)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 789,
		UserID:    456,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("QA Exception Channel: unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonExceptionChannel {
		t.Errorf("QA Exception Channel: expected ignore with exception_channel reason, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}

	t.Logf("✓ QA Exception Channel: Bot ignores mention in exception channel with reason=%q", decision.Reason)
}

func TestQAScenario_BotSelfMessage(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouterWithBotID(mockRepo, 999)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    999,
		Message: &domain.DiscordMessage{
			Content: "<@999> hello",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("QA Bot Self Message: unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonBotMessage {
		t.Errorf("QA Bot Self Message: expected ignore with bot_message reason, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}

	t.Logf("✓ QA Bot Self Message: Bot ignores own messages with reason=%q", decision.Reason)
}

func TestQAScenario_NoTrigger(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouterWithBotID(mockRepo, 999)

	event := &domain.DiscordEvent{
		Type:      "message_unknown",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "just a random message",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("QA No Trigger: unexpected error: %v", err)
	}

	if decision.Should || decision.Reason != ReasonNoTrigger {
		t.Errorf("QA No Trigger: expected ignore with no_trigger reason, got Should=%v Reason=%q", decision.Should, decision.Reason)
	}

	t.Logf("✓ QA No Trigger: Bot ignores non-trigger messages with reason=%q", decision.Reason)
}

func TestQAScenario_ExceptionChannelSuppressionPriority(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	mockRepo.AddException(123, 456)

	router := NewTriggerRouterWithBotID(mockRepo, 999)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "<@999> important message in exception channel",
		},
	}

	decision, err := router.Decide(context.Background(), event)

	if err != nil {
		t.Fatalf("QA Exception Priority: unexpected error: %v", err)
	}

	if decision.Should {
		t.Errorf("QA Exception Priority: exception channel should suppress even high-priority mention, got Should=%v", decision.Should)
	}

	if decision.Reason != ReasonExceptionChannel {
		t.Errorf("QA Exception Priority: expected exception_channel reason, got %q", decision.Reason)
	}

	t.Logf("✓ QA Exception Priority: Exception channel suppresses mention with reason=%q", decision.Reason)
}

func TestQAScenario_CaseInsensitiveIris(t *testing.T) {
	mockRepo := NewMockExceptionChannelRepo()
	router := NewTriggerRouterWithBotID(mockRepo, 999)

	testCases := []string{
		"hey IRIS, what's up?",
		"Hey Iris, tell me",
		"iris is cool",
		"IRIS IRIS IRIS",
	}

	for _, content := range testCases {
		event := &domain.DiscordEvent{
			Type:      "message_content",
			GuildID:   123,
			ChannelID: 456,
			UserID:    789,
			Message: &domain.DiscordMessage{
				Content: content,
			},
		}

		decision, err := router.Decide(context.Background(), event)

		if err != nil {
			t.Fatalf("QA Case Insensitive: unexpected error for %q: %v", content, err)
		}

		if !decision.Should || decision.Reason != ReasonNameMention {
			t.Errorf("QA Case Insensitive: expected respond for %q, got Should=%v Reason=%q", content, decision.Should, decision.Reason)
		}
	}

	t.Logf("✓ QA Case Insensitive: Bot responds to case-insensitive 'iris' mentions")
}
