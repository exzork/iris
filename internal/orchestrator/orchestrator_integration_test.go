package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/eko/iris-bot/internal/router"
)

type fakeGuildMemorySource struct {
	mu       sync.Mutex
	calls    int
	guildID  int64
	query    string
	results  []*repository.RecallResult
	err      error
}

func (f *fakeGuildMemorySource) Recall(ctx context.Context, guildID int64, query string) ([]*repository.RecallResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.guildID = guildID
	f.query = query
	return f.results, f.err
}

type fakeUserBehaviorSource struct {
	mu      sync.Mutex
	calls   int
	guildID int64
	userID  int64
	profile *domain.UserBehaviorProfile
	err     error
}

func (f *fakeUserBehaviorSource) Get(ctx context.Context, guildID int64, userID int64) (*domain.UserBehaviorProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.guildID = guildID
	f.userID = userID
	return f.profile, f.err
}

type integrationTestContextStore struct {
	mu       sync.Mutex
	messages []*domain.ChannelMessage
}

func (f *integrationTestContextStore) ListRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*domain.ChannelMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.messages, nil
}

func (f *integrationTestContextStore) GetByID(ctx context.Context, guildID, messageID int64) (*domain.ChannelMessage, error) {
	return nil, nil
}

func (f *integrationTestContextStore) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	return nil, nil
}

type integrationTestChannelCapture struct {
	mu       sync.Mutex
	captured []*domain.ChannelMessage
}

func (f *integrationTestChannelCapture) Capture(ctx context.Context, msg *domain.ChannelMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captured = append(f.captured, msg)
	return nil
}

type fakeAllowedQuerier struct {
	allowedChannels map[int64]bool
}

func (f *fakeAllowedQuerier) IsAllowed(ctx context.Context, guildID int64, channelID int64) (bool, error) {
	if f == nil || f.allowedChannels == nil {
		return false, nil
	}
	return f.allowedChannels[channelID], nil
}

func (f *fakeAllowedQuerier) ListByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	return []int64{}, nil
}

func (f *fakeAllowedQuerier) Add(ctx context.Context, guildID int64, channelID int64) error {
	return nil
}

func (f *fakeAllowedQuerier) Remove(ctx context.Context, guildID int64, channelID int64) error {
	return nil
}

func (f *fakeAllowedQuerier) HasAny(ctx context.Context, guildID int64) (bool, error) {
	return false, nil
}

type integrationTestDecider struct {
	shouldReply bool
}

func (f *integrationTestDecider) Decide(ctx context.Context, event *domain.DiscordEvent) (*router.Decision, error) {
	if f.shouldReply {
		return &router.Decision{Should: true, Reason: router.ReasonMention}, nil
	}
	return &router.Decision{Should: false, Reason: router.ReasonNoTrigger}, nil
}

type integrationTestLLMCaller struct {
	mu       sync.Mutex
	messages []map[string]string
}

func (f *integrationTestLLMCaller) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = messages
	return "test response", nil
}

func (f *integrationTestLLMCaller) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	return f.Chat(ctx, guildID, messages)
}

type integrationTestMessageSender struct {
	mu       sync.Mutex
	messages []string
}

func (f *integrationTestMessageSender) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, content)
	return nil
}

func TestOrchestrator_CaptureAllGuildMessages(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: false}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "hello world",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	capture.mu.Lock()
	defer capture.mu.Unlock()

	if len(capture.captured) != 1 {
		t.Fatalf("expected 1 captured message, got %d", len(capture.captured))
	}

	msg := capture.captured[0]
	if msg.GuildID != 12345 || msg.ChannelID != 67890 || msg.UserID != 111 {
		t.Fatalf("unexpected message metadata: guild=%d channel=%d user=%d", msg.GuildID, msg.ChannelID, msg.UserID)
	}
	if msg.Content != "hello world" {
		t.Fatalf("unexpected message content: %q", msg.Content)
	}
}

func TestOrchestrator_CapturePreservesAuthorName(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: false}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	authorOnMessage := "eko"
	authorOnEvent := "eko-event-fallback"

	cases := []struct {
		name             string
		messageAuthor    *string
		eventAuthor      *string
		wantAuthorName   *string
		shouldHaveAuthor bool
	}{
		{
			name:             "author from message",
			messageAuthor:    &authorOnMessage,
			eventAuthor:      nil,
			wantAuthorName:   &authorOnMessage,
			shouldHaveAuthor: true,
		},
		{
			name:             "fallback to event author",
			messageAuthor:    nil,
			eventAuthor:      &authorOnEvent,
			wantAuthorName:   &authorOnEvent,
			shouldHaveAuthor: true,
		},
		{
			name:             "nil author stays nil",
			messageAuthor:    nil,
			eventAuthor:      nil,
			wantAuthorName:   nil,
			shouldHaveAuthor: false,
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := &domain.DiscordEvent{
				GuildID:    100,
				ChannelID:  200,
				UserID:     int64(300 + i),
				AuthorName: tc.eventAuthor,
				Message: &domain.DiscordMessage{
					ID:         int64(900 + i),
					AuthorName: tc.messageAuthor,
					Content:    "msg " + tc.name,
				},
			}

			if err := orch.Enqueue(context.Background(), event); err != nil {
				t.Fatalf("enqueue: %v", err)
			}
			time.Sleep(200 * time.Millisecond)

			capture.mu.Lock()
			defer capture.mu.Unlock()

			var got *domain.ChannelMessage
			for _, m := range capture.captured {
				if m.MessageID == event.Message.ID {
					got = m
					break
				}
			}
			if got == nil {
				t.Fatalf("expected captured message for id %d", event.Message.ID)
			}

			if !tc.shouldHaveAuthor {
				if got.AuthorName != nil {
					t.Fatalf("expected nil AuthorName, got %q", *got.AuthorName)
				}
				return
			}
			if got.AuthorName == nil {
				t.Fatalf("expected AuthorName=%q, got nil", *tc.wantAuthorName)
			}
			if *got.AuthorName != *tc.wantAuthorName {
				t.Fatalf("expected AuthorName=%q, got %q", *tc.wantAuthorName, *got.AuthorName)
			}
		})
	}
}

func TestOrchestrator_NonTriggeringMessageCapturedButNoReply(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: false}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "non-triggering message",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	capture.mu.Lock()
	capturedCount := len(capture.captured)
	capture.mu.Unlock()

	if capturedCount != 1 {
		t.Fatalf("expected 1 captured message, got %d", capturedCount)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()

	if len(sender.messages) != 0 {
		t.Fatalf("expected no reply messages, got %d", len(sender.messages))
	}
}

func TestOrchestrator_GuildRecallInjectedIntoPrompt(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}
	guildMemory := &fakeGuildMemorySource{
		results: []*repository.RecallResult{
			{
				Message: &domain.ChannelMessage{
					MessageID: 100,
					Content:   "previous context about the topic",
				},
				Similarity: 0.95,
			},
		},
	}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		GuildMemory:  guildMemory,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "tell me about the topic",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	guildMemory.mu.Lock()
	recallCalls := guildMemory.calls
	recallGuildID := guildMemory.guildID
	guildMemory.mu.Unlock()

	if recallCalls != 1 {
		t.Fatalf("expected 1 recall call, got %d", recallCalls)
	}
	if recallGuildID != 12345 {
		t.Fatalf("expected recall for guild 12345, got %d", recallGuildID)
	}

	llmCaller.mu.Lock()
	defer llmCaller.mu.Unlock()

	if len(llmCaller.messages) == 0 {
		t.Fatalf("expected messages passed to LLM")
	}
}

func TestOrchestrator_UserBehaviorInjectedIntoPrompt(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}
	userBehavior := &fakeUserBehaviorSource{
		profile: &domain.UserBehaviorProfile{
			GuildID:                   12345,
			UserID:                    111,
			CommunicationStyle:        "casual",
			Formality:                 "low",
			ResponseLengthPreference:  "concise",
			FormattingPreference:      "bullet_points",
			RecurringTopics:           []string{"gaming", "anime"},
		},
	}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		UserBehavior: userBehavior,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "what do you think?",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	userBehavior.mu.Lock()
	behaviorCalls := userBehavior.calls
	behaviorGuildID := userBehavior.guildID
	behaviorUserID := userBehavior.userID
	userBehavior.mu.Unlock()

	if behaviorCalls != 1 {
		t.Fatalf("expected 1 behavior call, got %d", behaviorCalls)
	}
	if behaviorGuildID != 12345 {
		t.Fatalf("expected behavior lookup for guild 12345, got %d", behaviorGuildID)
	}
	if behaviorUserID != 111 {
		t.Fatalf("expected behavior lookup for user 111, got %d", behaviorUserID)
	}
}

func TestOrchestrator_DMExcludedFromGuildMemory(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}
	guildMemory := &fakeGuildMemorySource{}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		GuildMemory:  guildMemory,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   0,
		ChannelID: 0,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "DM message",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	guildMemory.mu.Lock()
	defer guildMemory.mu.Unlock()

	if guildMemory.calls != 0 {
		t.Fatalf("expected no recall calls for DM, got %d", guildMemory.calls)
	}
}

func TestOrchestrator_DMExcludedFromUserBehavior(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}
	userBehavior := &fakeUserBehaviorSource{}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		UserBehavior: userBehavior,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   0,
		ChannelID: 0,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "DM message",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	userBehavior.mu.Lock()
	defer userBehavior.mu.Unlock()

	if userBehavior.calls != 0 {
		t.Fatalf("expected no behavior calls for DM, got %d", userBehavior.calls)
	}
}

func TestOrchestrator_DMNotCaptured(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: false}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   0,
		ChannelID: 0,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "DM message",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	capture.mu.Lock()
	defer capture.mu.Unlock()

	if len(capture.captured) != 0 {
		t.Fatalf("expected no captured messages for DM, got %d", len(capture.captured))
	}
}

func TestOrchestrator_CaptureRespectsAllowlist(t *testing.T) {
	cases := []struct {
		name              string
		allowedQuerier    *fakeAllowedQuerier
		guildID           int64
		channelID         int64
		expectCaptured    int
		description       string
	}{
		{
			name: "allowlisted channel captures both user and bot messages",
			allowedQuerier: &fakeAllowedQuerier{
				allowedChannels: map[int64]bool{67890: true},
			},
			guildID:        12345,
			channelID:      67890,
			expectCaptured: 2,
			description:    "user message + bot reply",
		},
		{
			name: "non-allowlisted channel captures nothing",
			allowedQuerier: &fakeAllowedQuerier{
				allowedChannels: map[int64]bool{},
			},
			guildID:        12345,
			channelID:      67890,
			expectCaptured: 0,
			description:    "no captures when channel not in allowlist",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := &integrationTestChannelCapture{}
			decider := &integrationTestDecider{shouldReply: true}
			contextStore := &integrationTestContextStore{}
			llmCaller := &integrationTestLLMCaller{}
			sender := &integrationTestMessageSender{}

			cfg := Config{
				Router:         decider,
				LLM:            llmCaller,
				Discord:        sender,
				ContextStore:   contextStore,
				Capture:        capture,
				AllowedQuerier: tc.allowedQuerier,
				SystemPrompt:   "test system prompt",
				QueueSize:      128,
				WorkerCount:    1,
				JobTimeout:     5 * time.Second,
			}

			orch := New(cfg)
			orch.Start()
			defer orch.Stop()

			event := &domain.DiscordEvent{
				GuildID:   tc.guildID,
				ChannelID: tc.channelID,
				UserID:    111,
				Message: &domain.DiscordMessage{
					ID:      999,
					Content: "test message",
				},
			}

			err := orch.Enqueue(context.Background(), event)
			if err != nil {
				t.Fatalf("failed to enqueue: %v", err)
			}

			time.Sleep(1 * time.Second)

			capture.mu.Lock()
			defer capture.mu.Unlock()

			if len(capture.captured) != tc.expectCaptured {
				t.Fatalf("%s: expected %d captured messages, got %d", tc.description, tc.expectCaptured, len(capture.captured))
			}

			if tc.expectCaptured == 2 {
				if capture.captured[0].IsBot {
					t.Fatalf("first message should be user message (IsBot=false), got IsBot=true")
				}
				if capture.captured[0].Content != "test message" {
					t.Fatalf("first message content mismatch: got %q", capture.captured[0].Content)
				}

				if !capture.captured[1].IsBot {
					t.Fatalf("second message should be bot message (IsBot=true), got IsBot=false")
				}
				if capture.captured[1].Content != "test response" {
					t.Fatalf("second message content mismatch: got %q", capture.captured[1].Content)
				}
			}
		})
	}
}

func TestOrchestrator_NilAllowedQuerierCaptures(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	cfg := Config{
		Router:         decider,
		LLM:            llmCaller,
		Discord:        sender,
		ContextStore:   contextStore,
		Capture:        capture,
		AllowedQuerier: nil,
		SystemPrompt:   "test system prompt",
		QueueSize:      128,
		WorkerCount:    1,
		JobTimeout:     5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "test message",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(1 * time.Second)

	capture.mu.Lock()
	defer capture.mu.Unlock()

	if len(capture.captured) != 2 {
		t.Fatalf("expected 2 captured messages with nil AllowedQuerier (backward compat), got %d", len(capture.captured))
	}

	if capture.captured[0].IsBot {
		t.Fatalf("first message should be user message (IsBot=false), got IsBot=true")
	}
	if !capture.captured[1].IsBot {
		t.Fatalf("second message should be bot message (IsBot=true), got IsBot=false")
	}
}
