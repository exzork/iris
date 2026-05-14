package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/repository"
)

// Test 1: Guild messages captured without triggering reply
func TestMemoryIntegration_CaptureAllGuildMessagesNoReply(t *testing.T) {
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

	// Send a message that won't trigger a reply
	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "just a regular message",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify message was captured
	capture.mu.Lock()
	if len(capture.captured) != 1 {
		capture.mu.Unlock()
		t.Fatalf("expected 1 captured message, got %d", len(capture.captured))
	}
	msg := capture.captured[0]
	capture.mu.Unlock()

	if msg.GuildID != 12345 || msg.ChannelID != 67890 || msg.UserID != 111 {
		t.Fatalf("unexpected message metadata: guild=%d channel=%d user=%d", msg.GuildID, msg.ChannelID, msg.UserID)
	}
	if msg.Content != "just a regular message" {
		t.Fatalf("unexpected message content: %q", msg.Content)
	}

	// Verify no LLM call was made (no reply)
	llmCaller.mu.Lock()
	if len(llmCaller.messages) > 0 {
		llmCaller.mu.Unlock()
		t.Fatalf("expected no LLM call, but got one")
	}
	llmCaller.mu.Unlock()

	// Verify no message was sent
	sender.mu.Lock()
	if len(sender.messages) != 0 {
		sender.mu.Unlock()
		t.Fatalf("expected no messages sent, got %d", len(sender.messages))
	}
	sender.mu.Unlock()
}

// Test 2: Triggered messages get recall + behavior hints in prompt
func TestMemoryIntegration_TriggeredMessageIncludesRecallAndBehavior(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	// Setup guild memory source with recall results
	guildMemory := &fakeGuildMemorySource{
		results: []*repository.RecallResult{
			{
				Similarity: 0.85,
				Message: &domain.ChannelMessage{
					GuildID:   12345,
					ChannelID: 67890,
					MessageID: 1,
					UserID:    222,
					Content:   "previous discussion about topic X",
					IsBot:     false,
					CreatedAt: time.Now(),
				},
			},
		},
	}

	// Setup user behavior source with profile
	userBehavior := &fakeUserBehaviorSource{
		profile: &domain.UserBehaviorProfile{
			GuildID:                  12345,
			UserID:                   111,
			CommunicationStyle:       "casual and friendly",
			Formality:                "informal",
			ResponseLengthPreference: "concise",
			FormattingPreference:     "bullet points",
			RecurringTopics:          []string{"gaming", "anime"},
			EvidenceCount:            5,
		},
	}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		GuildMemory:  guildMemory,
		UserBehavior: userBehavior,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	// Send a message that will trigger a reply
	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "what do you think about topic X?",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify guild memory was called
	guildMemory.mu.Lock()
	if guildMemory.calls != 1 {
		guildMemory.mu.Unlock()
		t.Fatalf("expected 1 guild memory call, got %d", guildMemory.calls)
	}
	if guildMemory.guildID != 12345 {
		guildMemory.mu.Unlock()
		t.Fatalf("expected guild memory call with guildID 12345, got %d", guildMemory.guildID)
	}
	guildMemory.mu.Unlock()

	// Verify user behavior was called
	userBehavior.mu.Lock()
	if userBehavior.calls != 1 {
		userBehavior.mu.Unlock()
		t.Fatalf("expected 1 user behavior call, got %d", userBehavior.calls)
	}
	if userBehavior.guildID != 12345 || userBehavior.userID != 111 {
		userBehavior.mu.Unlock()
		t.Fatalf("expected user behavior call with guildID 12345 userID 111, got guild=%d user=%d", userBehavior.guildID, userBehavior.userID)
	}
	userBehavior.mu.Unlock()

	// Verify LLM was called with recall and behavior hints in messages
	llmCaller.mu.Lock()
	defer llmCaller.mu.Unlock()

	if len(llmCaller.messages) == 0 {
		t.Fatalf("expected LLM call with messages, got none")
	}

	// Find the recall and behavior blocks in the messages
	fullContent := ""
	for _, msg := range llmCaller.messages {
		fullContent += msg["content"] + "\n"
	}

	if !strings.Contains(fullContent, "UNTRUSTED SERVER MEMORY") {
		t.Fatalf("expected recall block in LLM messages, got: %s", fullContent)
	}

	if !strings.Contains(fullContent, "USER INTERACTION HINTS") {
		t.Fatalf("expected behavior hints block in LLM messages, got: %s", fullContent)
	}

	if !strings.Contains(fullContent, "previous discussion about topic X") {
		t.Fatalf("expected recalled message content in LLM messages, got: %s", fullContent)
	}

	if !strings.Contains(fullContent, "casual and friendly") {
		t.Fatalf("expected communication style hint in LLM messages, got: %s", fullContent)
	}
}

// Test 3: DMs (GuildID=0) do NOT call recall or behavior services
func TestMemoryIntegration_DMsExcludedFromRecallAndBehavior(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	// Setup guild memory and user behavior sources
	guildMemory := &fakeGuildMemorySource{}
	userBehavior := &fakeUserBehaviorSource{}

	cfg := Config{
		Router:       decider,
		LLM:          llmCaller,
		Discord:      sender,
		ContextStore: contextStore,
		Capture:      capture,
		GuildMemory:  guildMemory,
		UserBehavior: userBehavior,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	// Send a DM (GuildID=0)
	event := &domain.DiscordEvent{
		GuildID:   0, // DM
		ChannelID: 0,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "hello in DM",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify guild memory was NOT called
	guildMemory.mu.Lock()
	if guildMemory.calls != 0 {
		guildMemory.mu.Unlock()
		t.Fatalf("expected 0 guild memory calls for DM, got %d", guildMemory.calls)
	}
	guildMemory.mu.Unlock()

	// Verify user behavior was NOT called
	userBehavior.mu.Lock()
	if userBehavior.calls != 0 {
		userBehavior.mu.Unlock()
		t.Fatalf("expected 0 user behavior calls for DM, got %d", userBehavior.calls)
	}
	userBehavior.mu.Unlock()

	// Verify LLM was still called (reply should happen)
	llmCaller.mu.Lock()
	if len(llmCaller.messages) == 0 {
		llmCaller.mu.Unlock()
		t.Fatalf("expected LLM call for DM, got none")
	}
	llmCaller.mu.Unlock()

	// Verify no recall or behavior blocks in messages
	llmCaller.mu.Lock()
	defer llmCaller.mu.Unlock()

	fullContent := ""
	for _, msg := range llmCaller.messages {
		fullContent += msg["content"] + "\n"
	}

	if strings.Contains(fullContent, "UNTRUSTED SERVER MEMORY") {
		t.Fatalf("expected NO recall block in DM messages, but found one")
	}

	if strings.Contains(fullContent, "USER INTERACTION HINTS") {
		t.Fatalf("expected NO behavior hints block in DM messages, but found one")
	}
}

// Test 4: Verify cross-channel context still works with memory integration
func TestMemoryIntegration_CrossChannelContextWithMemory(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{
		messages: []*domain.ChannelMessage{
			{
				GuildID:   12345,
				ChannelID: 67890,
				MessageID: 1,
				UserID:    111,
				Content:   "current channel message",
				IsBot:     false,
				CreatedAt: time.Now(),
			},
		},
	}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	// Setup guild memory
	guildMemory := &fakeGuildMemorySource{
		results: []*repository.RecallResult{
			{
				Similarity: 0.80,
				Message: &domain.ChannelMessage{
					GuildID:   12345,
					ChannelID: 11111,
					MessageID: 2,
					UserID:    222,
					Content:   "recalled context",
					IsBot:     false,
					CreatedAt: time.Now(),
				},
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
			Content: "new message",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify guild memory was called
	guildMemory.mu.Lock()
	if guildMemory.calls != 1 {
		guildMemory.mu.Unlock()
		t.Fatalf("expected 1 guild memory call, got %d", guildMemory.calls)
	}
	guildMemory.mu.Unlock()

	// Verify LLM was called with both current channel and recalled context
	llmCaller.mu.Lock()
	defer llmCaller.mu.Unlock()

	if len(llmCaller.messages) == 0 {
		t.Fatalf("expected LLM call with messages, got none")
	}

	fullContent := ""
	for _, msg := range llmCaller.messages {
		fullContent += msg["content"] + "\n"
	}

	if !strings.Contains(fullContent, "current channel message") {
		t.Fatalf("expected current channel message in LLM messages")
	}

	if !strings.Contains(fullContent, "UNTRUSTED SERVER MEMORY") {
		t.Fatalf("expected recall block in LLM messages")
	}
}
