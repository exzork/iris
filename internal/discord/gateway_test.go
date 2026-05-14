package discord

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eko/iris-bot/internal/domain"
)

func TestBuildReplyMessage_AttachesReferenceAndAllowedMentions(t *testing.T) {
	send := buildReplyMessage(123, 456, 789, "hello", true)

	if send.Content != "hello" {
		t.Errorf("Content = %q, want %q", send.Content, "hello")
	}
	if send.Reference == nil {
		t.Fatal("expected Reference, got nil")
	}
	if send.Reference.MessageID != "789" {
		t.Errorf("Reference.MessageID = %q, want %q", send.Reference.MessageID, "789")
	}
	if send.Reference.ChannelID != "456" {
		t.Errorf("Reference.ChannelID = %q, want %q", send.Reference.ChannelID, "456")
	}
	if send.Reference.GuildID != "123" {
		t.Errorf("Reference.GuildID = %q, want %q", send.Reference.GuildID, "123")
	}
	if send.Reference.FailIfNotExists == nil || *send.Reference.FailIfNotExists {
		t.Errorf("Reference.FailIfNotExists should be false (do not fail on missing parent)")
	}
	if send.AllowedMentions == nil {
		t.Fatal("expected AllowedMentions, got nil")
	}
	if !send.AllowedMentions.RepliedUser {
		t.Errorf("expected RepliedUser=true (ping enabled)")
	}
	wantParse := []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers}
	if len(send.AllowedMentions.Parse) != len(wantParse) || send.AllowedMentions.Parse[0] != wantParse[0] {
		t.Errorf("AllowedMentions.Parse = %v, want %v", send.AllowedMentions.Parse, wantParse)
	}
	for _, mt := range send.AllowedMentions.Parse {
		if mt == discordgo.AllowedMentionTypeRoles || mt == discordgo.AllowedMentionTypeEveryone {
			t.Errorf("Parse must not include roles or everyone, got %v", mt)
		}
	}
}

func TestBuildReplyMessage_NoPingPreservesUserMentionParse(t *testing.T) {
	send := buildReplyMessage(1, 2, 3, "hi <@4>", false)

	if send.AllowedMentions == nil {
		t.Fatal("expected AllowedMentions even when ping disabled")
	}
	if send.AllowedMentions.RepliedUser {
		t.Errorf("expected RepliedUser=false")
	}
	foundUsers := false
	for _, mt := range send.AllowedMentions.Parse {
		if mt == discordgo.AllowedMentionTypeUsers {
			foundUsers = true
		}
	}
	if !foundUsers {
		t.Errorf("user mentions must remain parseable so explicit <@id> tags work")
	}
	if send.Content != "hi <@4>" {
		t.Errorf("explicit user mention must not be stripped, got %q", send.Content)
	}
}

func TestGatewayAdapterCallbackNonBlocking(t *testing.T) {
	callbackCalled := make(chan bool, 1)
	slowCallback := func(ctx context.Context, event *domain.DiscordEvent) error {
		time.Sleep(100 * time.Millisecond)
		callbackCalled <- true
		return nil
	}

	adapter, err := NewGatewayAdapter("test-token", 999, slowCallback)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.wg.Add(1)
	go adapter.processWorkQueue()

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			ID:        1,
			GuildID:   123,
			ChannelID: 456,
			UserID:    789,
			Content:   "test",
		},
	}

	start := time.Now()
	adapter.workQueue <- event
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("enqueue took too long: %v", elapsed)
	}

	select {
	case <-callbackCalled:
	case <-time.After(1 * time.Second):
		t.Fatal("callback was not called")
	}

	close(adapter.stopChan)
	adapter.wg.Wait()
}

func TestGatewayAdapterHandleMessageMention(t *testing.T) {
	eventReceived := make(chan *domain.DiscordEvent, 1)
	callback := func(ctx context.Context, event *domain.DiscordEvent) error {
		eventReceived <- event
		return nil
	}

	adapter, err := NewGatewayAdapter("test-token", 999, callback)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.wg.Add(1)
	go adapter.processWorkQueue()

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "789",
		},
		Content: "<@999> hello",
		Mentions: []*discordgo.User{
			{ID: "999"},
		},
		Timestamp: time.Now(),
	}

	adapter.handleMessage(msg)

	select {
	case event := <-eventReceived:
		if event.Type != "message_mention" {
			t.Errorf("expected type 'message_mention', got %q", event.Type)
		}
		if event.Message.Content != "<@999> hello" {
			t.Errorf("expected content '<@999> hello', got %q", event.Message.Content)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("callback was not called")
	}

	close(adapter.stopChan)
	adapter.wg.Wait()
}

func TestGatewayAdapterIgnoreBotMessages(t *testing.T) {
	eventReceived := make(chan *domain.DiscordEvent, 1)
	callback := func(ctx context.Context, event *domain.DiscordEvent) error {
		eventReceived <- event
		return nil
	}

	adapter, err := NewGatewayAdapter("test-token", 999, callback)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.wg.Add(1)
	go adapter.processWorkQueue()

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID: "999",
		},
		Content:   "bot response",
		Timestamp: time.Now(),
	}

	adapter.handleMessage(msg)

	select {
	case <-eventReceived:
		t.Fatal("expected no event for bot message")
	case <-time.After(100 * time.Millisecond):
	}

	close(adapter.stopChan)
	adapter.wg.Wait()
}

func TestGatewayAdapterCallbackError(t *testing.T) {
	callCount := 0
	callback := func(ctx context.Context, event *domain.DiscordEvent) error {
		callCount++
		return errors.New("callback error")
	}

	adapter, err := NewGatewayAdapter("test-token", 999, callback)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.wg.Add(1)
	go adapter.processWorkQueue()

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
	}

	adapter.workQueue <- event

	time.Sleep(100 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("expected callback to be called once, got %d", callCount)
	}

	close(adapter.stopChan)
	adapter.wg.Wait()
}

func TestNewGatewayAdapterDeclaresMessageContentIntent(t *testing.T) {
	callback := func(ctx context.Context, event *domain.DiscordEvent) error {
		return nil
	}

	adapter, err := NewGatewayAdapter("fake-token", 42, callback)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if adapter.session == nil {
		t.Fatal("expected session to be initialized")
	}

	hasMessageContent := adapter.session.Identify.Intents&discordgo.IntentMessageContent != 0
	if !hasMessageContent {
		t.Errorf("expected MessageContent intent to be set, got intents: %v", adapter.session.Identify.Intents)
	}

	hasGuildMessages := adapter.session.Identify.Intents&discordgo.IntentsGuildMessages != 0
	if !hasGuildMessages {
		t.Errorf("expected GuildMessages intent to be set")
	}

	hasGuildMessageTyping := adapter.session.Identify.Intents&discordgo.IntentsGuildMessageTyping != 0
	if !hasGuildMessageTyping {
		t.Errorf("expected GuildMessageTyping intent to be set")
	}
}

func TestSessionManagerAddGetRemove(t *testing.T) {
	sm := NewSessionManager()

	callback := func(ctx context.Context, event *domain.DiscordEvent) error {
		return nil
	}

	adapter, err := NewGatewayAdapter("test-token", 999, callback)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	guildID := int64(123)
	sm.AddAdapter(guildID, adapter)

	retrieved, exists := sm.GetAdapter(guildID)
	if !exists {
		t.Fatal("expected adapter to exist")
	}

	if retrieved != adapter {
		t.Error("expected same adapter instance")
	}

	sm.RemoveAdapter(guildID)

	_, exists = sm.GetAdapter(guildID)
	if exists {
		t.Fatal("expected adapter to be removed")
	}
}

func TestSessionManagerMultipleAdapters(t *testing.T) {
	sm := NewSessionManager()

	callback := func(ctx context.Context, event *domain.DiscordEvent) error {
		return nil
	}

	adapter1, _ := NewGatewayAdapter("token1", 999, callback)
	adapter2, _ := NewGatewayAdapter("token2", 999, callback)

	sm.AddAdapter(int64(111), adapter1)
	sm.AddAdapter(int64(222), adapter2)

	a1, _ := sm.GetAdapter(int64(111))
	a2, _ := sm.GetAdapter(int64(222))

	if a1 != adapter1 || a2 != adapter2 {
		t.Error("expected correct adapters for each guild")
	}
}

func TestNormalizeMessageCreateWithReplyMetadata(t *testing.T) {
	normalizer := NewEventNormalizer(999)

	msg := &discordgo.Message{
		ID:        "123456789",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID:       "789",
			Username: "testuser",
			Bot:      false,
		},
		Content: "reply to something",
		MessageReference: &discordgo.MessageReference{
			MessageID: "999888777",
			ChannelID: "456",
		},
		Timestamp: time.Now(),
	}

	event, err := normalizer.NormalizeMessageCreate(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.ReplyToMessageID == nil || *event.ReplyToMessageID != 999888777 {
		t.Errorf("expected ReplyToMessageID to be 999888777, got %v", event.ReplyToMessageID)
	}
	if event.ReplyToChannelID == nil || *event.ReplyToChannelID != 456 {
		t.Errorf("expected ReplyToChannelID to be 456, got %v", event.ReplyToChannelID)
	}
	if event.Message.ReplyToMessageID == nil || *event.Message.ReplyToMessageID != 999888777 {
		t.Errorf("expected Message.ReplyToMessageID to be 999888777, got %v", event.Message.ReplyToMessageID)
	}
	if event.Message.ReplyToChannelID == nil || *event.Message.ReplyToChannelID != 456 {
		t.Errorf("expected Message.ReplyToChannelID to be 456, got %v", event.Message.ReplyToChannelID)
	}
}

func TestNormalizeMessageCreateIgnoresOwnBot(t *testing.T) {
	normalizer := NewEventNormalizer(999)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID:       "999",
			Username: "iris",
			Bot:      true,
		},
		Content:   "iris response",
		Timestamp: time.Now(),
	}

	event, err := normalizer.NormalizeMessageCreate(msg)
	if err != ErrBotMessage {
		t.Fatalf("expected ErrBotMessage, got %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event for own bot message")
	}
}

func TestNormalizeMessageCreateWithAuthorName(t *testing.T) {
	normalizer := NewEventNormalizer(999)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID:       "789",
			Username: "alice",
			Bot:      false,
		},
		Content:   "hello",
		Timestamp: time.Now(),
	}

	event, err := normalizer.NormalizeMessageCreate(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.AuthorName == nil || *event.AuthorName != "alice" {
		t.Errorf("expected AuthorName to be 'alice', got %v", event.AuthorName)
	}
	if event.Message.AuthorName == nil || *event.Message.AuthorName != "alice" {
		t.Errorf("expected Message.AuthorName to be 'alice', got %v", event.Message.AuthorName)
	}
}

func TestNormalizeMessageCreateWithAttachments(t *testing.T) {
	normalizer := NewEventNormalizer(999)

	msg := &discordgo.Message{
		ID:        "msg123",
		GuildID:   "123",
		ChannelID: "456",
		Author: &discordgo.User{
			ID:       "789",
			Username: "testuser",
			Bot:      false,
		},
		Content: "message with attachments",
		Attachments: []*discordgo.MessageAttachment{
			{ID: "att1", URL: "http://example.com/1", Size: 1024},
			{ID: "att2", URL: "http://example.com/2", Size: 2048},
		},
		Timestamp: time.Now(),
	}

	event, err := normalizer.NormalizeMessageCreate(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.AttachmentCount != 2 {
		t.Errorf("expected AttachmentCount to be 2, got %d", event.AttachmentCount)
	}
	if event.Message.AttachmentCount != 2 {
		t.Errorf("expected Message.AttachmentCount to be 2, got %d", event.Message.AttachmentCount)
	}
}

func TestProcessWorkQueueLogsCallbackError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	callback := func(ctx context.Context, event *domain.DiscordEvent) error {
		return errors.New("test callback error")
	}

	adapter, err := NewGatewayAdapter("test-token", 999, callback)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	adapter.logger = logger

	adapter.wg.Add(1)
	go adapter.processWorkQueue()

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			ID:        1,
			GuildID:   123,
			ChannelID: 456,
			UserID:    789,
			Content:   "test",
		},
	}

	adapter.workQueue <- event

	time.Sleep(100 * time.Millisecond)

	close(adapter.stopChan)
	adapter.wg.Wait()

	logOutput := buf.String()
	if logOutput == "" {
		t.Fatal("expected log output, got empty")
	}

	if !bytes.Contains(buf.Bytes(), []byte("callback failed")) {
		t.Errorf("expected 'callback failed' in log output, got: %s", logOutput)
	}

	if !bytes.Contains(buf.Bytes(), []byte("test callback error")) {
		t.Errorf("expected error message in log output, got: %s", logOutput)
	}
}
