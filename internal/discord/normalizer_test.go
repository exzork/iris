package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

// TestNormalizeMessageEventMention tests mention detection.
func TestNormalizeMessageEventMention(t *testing.T) {
	botID := int64(999)
	guildID := int64(123)
	channelID := int64(456)
	userID := int64(789)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "789",
		},
		Content: "<@999> hello iris",
		Mentions: []*discordgo.User{
			{ID: "999"},
		},
	}

	normalizer := NewEventNormalizer(botID)
	event, err := normalizer.NormalizeMessageCreate(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Type != "message_mention" {
		t.Errorf("expected type 'message_mention', got %q", event.Type)
	}

	if event.GuildID != guildID {
		t.Errorf("expected guildID %d, got %d", guildID, event.GuildID)
	}

	if event.ChannelID != channelID {
		t.Errorf("expected channelID %d, got %d", channelID, event.ChannelID)
	}

	if event.UserID != userID {
		t.Errorf("expected userID %d, got %d", userID, event.UserID)
	}

	if event.Message == nil {
		t.Fatal("expected Message to be set")
	}

	if event.Message.Content != msg.Content {
		t.Errorf("expected content %q, got %q", msg.Content, event.Message.Content)
	}
}

// TestNormalizeMessageEventReply tests reply-to-bot detection.
func TestNormalizeMessageEventReply(t *testing.T) {
	botID := int64(999)
	guildID := int64(123)
	userID := int64(789)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "789",
		},
		Content: "reply to bot",
		MessageReference: &discordgo.MessageReference{
			MessageID: "msg999",
		},
	}

	normalizer := NewEventNormalizer(botID)
	normalizer.SetReferencedMessage("msg999", &discordgo.Message{
		ID: "msg999",
		Author: &discordgo.User{
			ID: "999",
		},
	})

	event, err := normalizer.NormalizeMessageCreate(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Type != "message_reply" {
		t.Errorf("expected type 'message_reply', got %q", event.Type)
	}

	if event.GuildID != guildID {
		t.Errorf("expected guildID %d, got %d", guildID, event.GuildID)
	}

	if event.UserID != userID {
		t.Errorf("expected userID %d, got %d", userID, event.UserID)
	}
}

// TestNormalizeMessageEventContent tests content-based trigger.
func TestNormalizeMessageEventContent(t *testing.T) {
	botID := int64(999)
	guildID := int64(123)
	userID := int64(789)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "789",
		},
		Content: "hey iris, what's up?",
	}

	normalizer := NewEventNormalizer(botID)
	event, err := normalizer.NormalizeMessageCreate(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Type != "message_content" {
		t.Errorf("expected type 'message_content', got %q", event.Type)
	}

	if event.GuildID != guildID {
		t.Errorf("expected guildID %d, got %d", guildID, event.GuildID)
	}

	if event.UserID != userID {
		t.Errorf("expected userID %d, got %d", userID, event.UserID)
	}
}

// TestNormalizeMessageEventMissingContent tests missing Message Content Intent fallback.
func TestNormalizeMessageEventMissingContent(t *testing.T) {
	botID := int64(999)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "789",
		},
		Content: "", // Empty content due to missing intent
	}

	normalizer := NewEventNormalizer(botID)
	event, err := normalizer.NormalizeMessageCreate(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still normalize, but content will be empty
	if event.Message == nil {
		t.Fatal("expected Message to be set even with missing content")
	}

	if event.Message.Content != "" {
		t.Errorf("expected empty content, got %q", event.Message.Content)
	}
}

// TestNormalizeMessageEventIgnoreBotMessages tests that bot messages are ignored.
func TestNormalizeMessageEventIgnoreBotMessages(t *testing.T) {
	botID := int64(999)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "999", // Bot's own ID
		},
		Content: "bot response",
	}

	normalizer := NewEventNormalizer(botID)
	event, err := normalizer.NormalizeMessageCreate(msg)

	if err != ErrBotMessage {
		t.Errorf("expected ErrBotMessage, got %v", err)
	}

	if event != nil {
		t.Errorf("expected nil event for bot message, got %v", event)
	}
}

// TestNormalizeMessageEventAttachments tests attachment handling.
func TestNormalizeMessageEventAttachments(t *testing.T) {
	botID := int64(999)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "789",
		},
		Content: "check this out",
		Attachments: []*discordgo.MessageAttachment{
			{
				ID:   "att1",
				URL:  "https://example.com/image.png",
				Size: 1024,
			},
		},
		Mentions: []*discordgo.User{
			{ID: "999"},
		},
	}

	normalizer := NewEventNormalizer(botID)
	event, err := normalizer.NormalizeMessageCreate(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.Type != "message_mention" {
		t.Errorf("expected type 'message_mention', got %q", event.Type)
	}

	if event.Message == nil || len(event.Message.Attachments) == 0 {
		t.Fatal("expected attachments to be preserved")
	}

	att, ok := event.Message.Attachments[0].(MessageAttachment)
	if !ok {
		t.Fatal("expected attachment to be MessageAttachment type")
	}

	if att.URL != "https://example.com/image.png" {
		t.Errorf("expected attachment URL, got %q", att.URL)
	}
}

// TestNormalizeMessageEventPriority tests event type priority (mention > reply > content).
func TestNormalizeMessageEventPriority(t *testing.T) {
	botID := int64(999)

	// Message with both mention and reply
	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "789",
		},
		Content: "<@999> reply to bot",
		Mentions: []*discordgo.User{
			{ID: "999"},
		},
		MessageReference: &discordgo.MessageReference{
			MessageID: "msg999",
		},
	}

	normalizer := NewEventNormalizer(botID)
	normalizer.SetReferencedMessage("msg999", &discordgo.Message{
		ID: "msg999",
		Author: &discordgo.User{
			ID: "999",
		},
	})

	event, err := normalizer.NormalizeMessageCreate(msg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mention should take priority
	if event.Type != "message_mention" {
		t.Errorf("expected type 'message_mention' (priority), got %q", event.Type)
	}
}

// TestEventNormalizerContract verifies the normalizer implements expected behavior.
func TestEventNormalizerContract(t *testing.T) {
	botID := int64(999)
	normalizer := NewEventNormalizer(botID)

	if normalizer == nil {
		t.Fatal("NewEventNormalizer returned nil")
	}

	// Verify it can handle nil message gracefully
	event, err := normalizer.NormalizeMessageCreate(nil)
	if err == nil {
		t.Error("expected error for nil message")
	}
	if event != nil {
		t.Error("expected nil event for nil message")
	}
}
