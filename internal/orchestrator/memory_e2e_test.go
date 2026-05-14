package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/repository"
)

// TestServerMemoryE2E_GuildRecallReachesLLM verifies that guild messages are captured
// and recalled memories reach the LLM prompt with the [UNTRUSTED SERVER MEMORY] block.
func TestServerMemoryE2E_GuildRecallReachesLLM(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	// Setup guild memory with a recalled result
	guildMemory := &fakeGuildMemorySource{
		results: []*repository.RecallResult{
			{
				Similarity: 0.85,
				Message: &domain.ChannelMessage{
					GuildID:   12345,
					ChannelID: 67890,
					MessageID: 1,
					UserID:    222,
					Content:   "previous discussion about server memory",
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

	// Send a message that triggers a reply
	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "what was discussed before?",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify guild memory was called with correct guild ID
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

	// Verify LLM was called with recall block
	llmCaller.mu.Lock()
	defer llmCaller.mu.Unlock()

	if len(llmCaller.messages) == 0 {
		t.Fatalf("expected LLM call with messages, got none")
	}

	fullContent := ""
	for _, msg := range llmCaller.messages {
		fullContent += msg["content"] + "\n"
	}

	if !strings.Contains(fullContent, "UNTRUSTED SERVER MEMORY") {
		t.Fatalf("expected [UNTRUSTED SERVER MEMORY] block in LLM messages, got: %s", fullContent)
	}

	if !strings.Contains(fullContent, "previous discussion about server memory") {
		t.Fatalf("expected recalled message content in LLM messages, got: %s", fullContent)
	}
}

// TestServerMemoryE2E_UserBehaviorHintsReachLLM verifies that user behavior profiles
// reach the LLM prompt with the [USER INTERACTION HINTS] block.
func TestServerMemoryE2E_UserBehaviorHintsReachLLM(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	// Setup user behavior with a profile
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
		UserBehavior: userBehavior,
		SystemPrompt: "test system prompt",
		QueueSize:    128,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	// Send a message that triggers a reply
	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "hey, what's up?",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify user behavior was called with correct guild and user IDs
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

	// Verify LLM was called with behavior hints block
	llmCaller.mu.Lock()
	defer llmCaller.mu.Unlock()

	if len(llmCaller.messages) == 0 {
		t.Fatalf("expected LLM call with messages, got none")
	}

	fullContent := ""
	for _, msg := range llmCaller.messages {
		fullContent += msg["content"] + "\n"
	}

	if !strings.Contains(fullContent, "USER INTERACTION HINTS") {
		t.Fatalf("expected [USER INTERACTION HINTS] block in LLM messages, got: %s", fullContent)
	}

	if !strings.Contains(fullContent, "casual and friendly") {
		t.Fatalf("expected communication style hint in LLM messages, got: %s", fullContent)
	}
}

// TestServerMemoryE2E_CrossGuildIsolation verifies that guild A's messages do not
// appear in guild B's recall results.
func TestServerMemoryE2E_CrossGuildIsolation(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	// Setup guild memory that will be called
	guildMemory := &fakeGuildMemorySource{
		results: []*repository.RecallResult{
			{
				Similarity: 0.80,
				Message: &domain.ChannelMessage{
					GuildID:   99999, // Different guild
					ChannelID: 11111,
					MessageID: 1,
					UserID:    222,
					Content:   "guild A message",
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

	// Send a message in guild B (12345)
	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111,
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "message in guild B",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify guild memory was called with guild B's ID, not guild A's
	guildMemory.mu.Lock()
	if guildMemory.calls != 1 {
		guildMemory.mu.Unlock()
		t.Fatalf("expected 1 guild memory call, got %d", guildMemory.calls)
	}
	if guildMemory.guildID != 12345 {
		guildMemory.mu.Unlock()
		t.Fatalf("expected guild memory call with guildID 12345 (guild B), got %d", guildMemory.guildID)
	}
	guildMemory.mu.Unlock()
}

// TestServerMemoryE2E_CrossUserBehaviorIsolation verifies that user A's behavior
// profile does not appear for user B.
func TestServerMemoryE2E_CrossUserBehaviorIsolation(t *testing.T) {
	capture := &integrationTestChannelCapture{}
	decider := &integrationTestDecider{shouldReply: true}
	contextStore := &integrationTestContextStore{}
	llmCaller := &integrationTestLLMCaller{}
	sender := &integrationTestMessageSender{}

	// Setup user behavior that will be called
	userBehavior := &fakeUserBehaviorSource{
		profile: &domain.UserBehaviorProfile{
			GuildID:                  12345,
			UserID:                   999, // Different user
			CommunicationStyle:       "user A style",
			Formality:                "formal",
			ResponseLengthPreference: "verbose",
			FormattingPreference:     "paragraphs",
			RecurringTopics:          []string{"topic A"},
			EvidenceCount:            10,
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

	// Send a message from user B (111)
	event := &domain.DiscordEvent{
		GuildID:   12345,
		ChannelID: 67890,
		UserID:    111, // User B
		Message: &domain.DiscordMessage{
			ID:      999,
			Content: "message from user B",
		},
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify user behavior was called with user B's ID, not user A's
	userBehavior.mu.Lock()
	if userBehavior.calls != 1 {
		userBehavior.mu.Unlock()
		t.Fatalf("expected 1 user behavior call, got %d", userBehavior.calls)
	}
	if userBehavior.userID != 111 {
		userBehavior.mu.Unlock()
		t.Fatalf("expected user behavior call with userID 111 (user B), got %d", userBehavior.userID)
	}
	userBehavior.mu.Unlock()
}

// TestServerMemoryE2E_DMsExcluded verifies that DM events (GuildID=0) do not
// invoke recall or behavior services.
func TestServerMemoryE2E_DMsExcluded(t *testing.T) {
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
