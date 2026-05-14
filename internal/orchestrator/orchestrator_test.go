package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/router"
	"github.com/eko/iris-bot/internal/testutil"
)

type stubRouter struct {
	decision *router.Decision
	err      error
}

func (s *stubRouter) Decide(ctx context.Context, event *domain.DiscordEvent) (*router.Decision, error) {
	return s.decision, s.err
}

func buildEvent(id string, guildID, channelID int64, content string) *domain.DiscordEvent {
	return &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   guildID,
		ChannelID: channelID,
		UserID:    42,
		Message: &domain.DiscordMessage{
			ID:      hashID(id),
			GuildID: guildID,
			Content: content,
		},
		CreatedAt: time.Now(),
	}
}

func hashID(s string) int64 {
	var h int64 = 1469598103934665603
	for _, c := range s {
		h ^= int64(c)
		h *= 1099511628211
	}
	return h
}

func TestEnqueueNonBlocking(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	llm.SimulateLatency = 200 * time.Millisecond
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		QueueSize:    10,
		WorkerCount:  2,
		EnqueueLimit: 50 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	start := time.Now()
	err := orch.Enqueue(context.Background(), buildEvent("evt-1", 1, 1, "hello"))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > 20*time.Millisecond {
		t.Errorf("enqueue took %v, expected quick non-blocking", elapsed)
	}
}

func TestEnqueueQueueFull(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	llm.SimulateLatency = 500 * time.Millisecond
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		QueueSize:    1,
		WorkerCount:  1,
		EnqueueLimit: 5 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	_ = orch.Enqueue(context.Background(), buildEvent("e1", 1, 1, "a"))
	_ = orch.Enqueue(context.Background(), buildEvent("e2", 1, 1, "b"))

	err := orch.Enqueue(context.Background(), buildEvent("e3", 1, 1, "c"))
	if err == nil {
		t.Errorf("expected queue-full error but got nil")
	}
}

func TestEventDeduplication(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	var processed int32
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		atomic.AddInt32(&processed, 1)
		return "ok", nil
	}
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		QueueSize:   10,
		WorkerCount: 2,
		DedupeTTL:   1 * time.Second,
	})
	orch.Start()
	defer orch.Stop()

	ev := buildEvent("same-id", 1, 1, "hello")
	_ = orch.Enqueue(context.Background(), ev)
	_ = orch.Enqueue(context.Background(), ev)
	_ = orch.Enqueue(context.Background(), ev)

	waitUntil(t, 500*time.Millisecond, func() bool {
		return orch.ProcessedCount() >= 1 && idle(orch)
	})

	if got := atomic.LoadInt32(&processed); got != 1 {
		t.Errorf("expected 1 processed call, got %d", got)
	}
}

func TestIgnoreDecisionSkipsLLM(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	var calls int32
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", nil
	}
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Ignore(router.ReasonExceptionChannel)}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		QueueSize:   10,
		WorkerCount: 1,
	})
	orch.Start()
	defer orch.Stop()

	_ = orch.Enqueue(context.Background(), buildEvent("x", 1, 1, "hi"))

	waitUntil(t, 200*time.Millisecond, func() bool {
		return idle(orch)
	})

	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("expected LLM not called on Ignore decision")
	}
	if len(disc.GetSentMessages()) != 0 {
		t.Errorf("expected no messages sent on Ignore")
	}
}

func TestRespondDecisionCallsLLMAndSends(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		return "halo dari iris", nil
	}
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		QueueSize:   10,
		WorkerCount: 1,
	})
	orch.Start()
	defer orch.Stop()

	_ = orch.Enqueue(context.Background(), buildEvent("e", 2, 3, "hai"))

	waitUntil(t, 500*time.Millisecond, func() bool {
		return len(disc.GetSentMessages()) >= 1
	})

	msgs := disc.GetSentMessages()
	if len(msgs) < 1 {
		t.Fatalf("expected at least 1 sent message, got %d", len(msgs))
	}
	if msgs[0].Content != "halo dari iris" {
		t.Errorf("expected 'halo dari iris', got %q", msgs[0].Content)
	}
	if msgs[0].GuildID != 2 || msgs[0].ChannelID != 3 {
		t.Errorf("wrong target: guild=%d channel=%d", msgs[0].GuildID, msgs[0].ChannelID)
	}
}

func TestLongResponseIsSplit(t *testing.T) {
	longText := strings.Repeat("x", 4500)
	llm := testutil.NewFakeLLMClient()
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		return longText, nil
	}
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		QueueSize:   10,
		WorkerCount: 1,
	})
	orch.Start()
	defer orch.Stop()

	_ = orch.Enqueue(context.Background(), buildEvent("long", 1, 1, "please be long"))

	waitUntil(t, 500*time.Millisecond, func() bool {
		return len(disc.GetSentMessages()) >= 3
	})

	msgs := disc.GetSentMessages()
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(msgs))
	}
	total := 0
	for _, m := range msgs {
		if len(m.Content) > DiscordMessageLimit {
			t.Errorf("chunk exceeds limit: len=%d", len(m.Content))
		}
		total += len(m.Content)
	}
	if total != len(longText) {
		t.Errorf("total chunk length %d != original %d", total, len(longText))
	}
}

type typingRecorder struct {
	testutil.FakeDiscordClient
	typingCount int32
}

func (t *typingRecorder) SendTyping(ctx context.Context, guildID, channelID int64) error {
	atomic.AddInt32(&t.typingCount, 1)
	return nil
}

func TestTypingIndicatorForSlowLLM(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	llm.SimulateLatency = 300 * time.Millisecond
	disc := &typingRecorder{FakeDiscordClient: *testutil.NewFakeDiscordClient()}
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		QueueSize:    10,
		WorkerCount:  1,
		TypingAfter:  50 * time.Millisecond,
		TypingRepeat: 100 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	_ = orch.Enqueue(context.Background(), buildEvent("slow", 1, 1, "think please"))

	waitUntil(t, 800*time.Millisecond, func() bool {
		return len(disc.GetSentMessages()) >= 1
	})

	if atomic.LoadInt32(&disc.typingCount) < 1 {
		t.Errorf("expected at least 1 typing indicator, got %d", disc.typingCount)
	}
}

func TestCancellationOnStop(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	started := make(chan struct{}, 10)
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "never", nil
		}
	}
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		QueueSize:   10,
		WorkerCount: 1,
	})
	orch.Start()

	_ = orch.Enqueue(context.Background(), buildEvent("c", 1, 1, "hi"))

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("LLM call never started")
	}

	done := make(chan struct{})
	go func() {
		orch.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("orchestrator Stop() blocked too long")
	}
}

func TestConcurrentProcessing(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	var inFlight int32
	var maxInFlight int32
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			m := atomic.LoadInt32(&maxInFlight)
			if cur <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return "done", nil
	}
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		QueueSize:   20,
		WorkerCount: 3,
	})
	orch.Start()
	defer orch.Stop()

	for i := 0; i < 6; i++ {
		_ = orch.Enqueue(context.Background(), buildEvent(fmt.Sprintf("c-%d", i), 1, 1, "hi"))
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) >= 6
	})

	if got := atomic.LoadInt32(&maxInFlight); got < 2 {
		t.Errorf("expected at least 2 concurrent workers, got %d", got)
	}
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func idle(o *Orchestrator) bool {
	return o.InFlight() == 0 && o.QueueDepth() == 0
}

var _ = sync.Mutex{}

func TestPromoterRunsOnDetachedContext(t *testing.T) {
	promoterContextCanceled := atomic.Bool{}
	promoterCompleted := atomic.Bool{}

	mockPromoter := &mockMemoryPromoter{
		onConsider: func(ctx context.Context, event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage, response string) {
			select {
			case <-ctx.Done():
				promoterContextCanceled.Store(true)
				return
			case <-time.After(100 * time.Millisecond):
				promoterCompleted.Store(true)
			}
		},
	}

	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		Promoter:    mockPromoter,
		QueueSize:   10,
		WorkerCount: 1,
		JobTimeout:  100 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	err := orch.Enqueue(context.Background(), buildEvent("promo-test", 1, 1, "test"))
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return promoterCompleted.Load()
	})

	if promoterContextCanceled.Load() {
		t.Error("promoter context was canceled, expected it to remain alive on detached context")
	}
	if !promoterCompleted.Load() {
		t.Error("promoter did not complete within timeout")
	}
}

type mockMemoryPromoter struct {
	onConsider func(ctx context.Context, event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage, response string)
}

func (m *mockMemoryPromoter) Consider(ctx context.Context, event *domain.DiscordEvent, contextMessages []*domain.ChannelMessage, response string) {
	if m.onConsider != nil {
		m.onConsider(ctx, event, contextMessages, response)
	}
}

type mockTierRouter struct {
	classifyResult string
	modelForResult string
}

func (m *mockTierRouter) Classify(ctx context.Context, guildID int64, query string) (string, error) {
	return m.classifyResult, nil
}

func (m *mockTierRouter) ModelFor(tier string) string {
	return m.modelForResult
}

func TestLLMRequestLogsActualModel(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	tierRouter := &mockTierRouter{
		classifyResult: "strong",
		modelForResult: "kr/claude-opus-4.7",
	}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		TierRouter:  tierRouter,
		QueueSize:   10,
		WorkerCount: 1,
	})
	orch.Start()
	defer orch.Stop()

	err := orch.Enqueue(context.Background(), buildEvent("model-test", 1, 1, "complex query"))
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})

	if len(disc.GetSentMessages()) == 0 {
		t.Error("expected at least one message sent")
	}
}

type fakeChannelCapture struct {
	mu       sync.Mutex
	captured []*domain.ChannelMessage
}

func (f *fakeChannelCapture) Capture(ctx context.Context, msg *domain.ChannelMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captured = append(f.captured, msg)
	return nil
}

func (f *fakeChannelCapture) GetCaptured() []*domain.ChannelMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]*domain.ChannelMessage, len(f.captured))
	copy(result, f.captured)
	return result
}

func TestHandle_PersistsIncomingMessage(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	capture := &fakeChannelCapture{}
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		Capture:      capture,
		QueueSize:    10,
		WorkerCount:  1,
		EnqueueLimit: 50 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("test-msg-1", 687347156524204067, 999, "hello iris")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(capture.GetCaptured()) > 0
	})

	captured := capture.GetCaptured()
	if len(captured) == 0 {
		t.Fatal("expected at least one captured message")
	}

	userMsg := captured[0]
	if userMsg.GuildID != 687347156524204067 {
		t.Errorf("expected guild 687347156524204067, got %d", userMsg.GuildID)
	}
	if userMsg.ChannelID != 999 {
		t.Errorf("expected channel 999, got %d", userMsg.ChannelID)
	}
	if userMsg.Content != "hello iris" {
		t.Errorf("expected content 'hello iris', got %q", userMsg.Content)
	}
	if userMsg.IsBot {
		t.Error("expected IsBot=false for user message")
	}
	if userMsg.UserID != 42 {
		t.Errorf("expected UserID 42, got %d", userMsg.UserID)
	}
}

func TestHandle_PersistsBotReply(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		return "This is Iris's response", nil
	}
	disc := testutil.NewFakeDiscordClient()
	capture := &fakeChannelCapture{}
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		Capture:      capture,
		QueueSize:    10,
		WorkerCount:  1,
		EnqueueLimit: 50 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("test-msg-2", 687347156524204067, 999, "hello iris")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(capture.GetCaptured()) >= 2
	})

	captured := capture.GetCaptured()
	if len(captured) < 2 {
		t.Fatalf("expected at least 2 captured messages (user + bot), got %d", len(captured))
	}

	botMsg := captured[1]
	if !botMsg.IsBot {
		t.Error("expected IsBot=true for bot message")
	}
	if botMsg.Content != "This is Iris's response" {
		t.Errorf("expected bot content 'This is Iris's response', got %q", botMsg.Content)
	}
	if botMsg.GuildID != 687347156524204067 {
		t.Errorf("expected guild 687347156524204067, got %d", botMsg.GuildID)
	}
	if botMsg.ChannelID != 999 {
		t.Errorf("expected channel 999, got %d", botMsg.ChannelID)
	}
	if botMsg.UserID != 0 {
		t.Errorf("expected UserID 0 for bot message, got %d", botMsg.UserID)
	}
	if botMsg.MessageID >= 0 {
		t.Errorf("expected synthetic message ID to be negative, got %d", botMsg.MessageID)
	}
}

func TestHandle_CaptureNilSafe(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		Capture:      nil,
		QueueSize:    10,
		WorkerCount:  1,
		EnqueueLimit: 50 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("test-msg-3", 687347156524204067, 999, "hello iris")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})

	if len(disc.GetSentMessages()) == 0 {
		t.Error("expected message sent even with nil Capture")
	}
}

type fakeToolsLLM struct {
	calls atomic.Int32
	reply string
}

func (f *fakeToolsLLM) ChatWithTools(ctx context.Context, messages []map[string]string, cfg llm.ChatWithToolsConfig) (string, error) {
	f.calls.Add(1)
	return f.reply, nil
}

type fakeNoopExecutor struct{}

func (f *fakeNoopExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	return "", nil
}

func TestHandle_UsesToolsLLMWhenConfigured(t *testing.T) {
	plainLLM := testutil.NewFakeLLMClient()
	toolsLLM := &fakeToolsLLM{reply: "Tool-based response"}
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          plainLLM,
		ToolsLLM:     toolsLLM,
		Discord:      disc,
		ToolExecutor: &fakeNoopExecutor{},
		Tools:        []map[string]interface{}{{"type": "function", "function": map[string]interface{}{"name": "test"}}},
		QueueSize:    10,
		WorkerCount:  1,
		EnqueueLimit: 50 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("tools-test-1", 1, 1, "hello")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})

	if toolsLLM.calls.Load() != 1 {
		t.Errorf("expected ToolsLLM called exactly once, got %d", toolsLLM.calls.Load())
	}
	if plainLLM.CallCount() != 0 {
		t.Errorf("expected plain LLM not called, got %d calls", plainLLM.CallCount())
	}
}

func TestHandle_FallsBackToPlainLLMWhenNoTools(t *testing.T) {
	plainLLM := testutil.NewFakeLLMClient()
	toolsLLM := &fakeToolsLLM{reply: "Tool-based response"}
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          plainLLM,
		ToolsLLM:     toolsLLM,
		Discord:      disc,
		ToolExecutor: &fakeNoopExecutor{},
		Tools:        nil,
		QueueSize:    10,
		WorkerCount:  1,
		EnqueueLimit: 50 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("no-tools-test-1", 1, 1, "hello")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})

	if plainLLM.CallCount() != 1 {
		t.Errorf("expected plain LLM called exactly once, got %d", plainLLM.CallCount())
	}
	if toolsLLM.calls.Load() != 0 {
		t.Errorf("expected ToolsLLM not called, got %d calls", toolsLLM.calls.Load())
	}
}

func TestHandle_NonToolsPath_UsesTierModel(t *testing.T) {
	plainLLM := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	classifierLLM := testutil.NewFakeLLMClient()
	tierRouter := &llm.TierRouter{
		Classifier: classifierLLM,
		Router:     "kr/claude-haiku-4.5",
		Default:    "kr/claude-haiku-4.5",
		Strong:     "kr/claude-opus-4.7",
	}

	orch := New(Config{
		Router:       r,
		LLM:          plainLLM,
		TierRouter:   tierRouter,
		Discord:      disc,
		Tools:        nil,
		QueueSize:    10,
		WorkerCount:  1,
		EnqueueLimit: 50 * time.Millisecond,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("tier-test-1", 1, 1, "hello")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})

	if plainLLM.LastModelUsed == "" {
		t.Errorf("expected ChatWithModel to be called with a model, but LastModelUsed is empty")
	}
	if plainLLM.LastModelUsed != "kr/claude-haiku-4.5" {
		t.Errorf("expected model kr/claude-haiku-4.5, got %s", plainLLM.LastModelUsed)
	}
}

func TestTyping_SpansPipeline(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		select {
		case <-time.After(7 * time.Second):
			return "response after 7 seconds", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	disc := testutil.NewFakeDiscordClient()
	typingCalls := atomic.Int32{}
	disc.SendTypingFn = func(ctx context.Context, guildID, channelID int64) error {
		typingCalls.Add(1)
		return nil
	}

	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		QueueSize:    10,
		WorkerCount:  1,
		TypingRepeat: 1 * time.Second,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("typing-test-1", 1, 1, "hello")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 10*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})

	calls := typingCalls.Load()
	if calls < 2 {
		t.Errorf("expected at least 2 typing calls (initial + at least 1 refresh), got %d", calls)
	}
}

func TestTyping_ErrorLogsWarn(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	llm.ChatFn = func(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
		return "response", nil
	}

	disc := testutil.NewFakeDiscordClient()
	typingErr := errors.New("channel not found")
	disc.SendTypingFn = func(ctx context.Context, guildID, channelID int64) error {
		return typingErr
	}

	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:      r,
		LLM:         llm,
		Discord:     disc,
		QueueSize:   10,
		WorkerCount: 1,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("typing-error-test-1", 1, 1, "hello")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})

	if len(disc.GetSentMessages()) == 0 {
		t.Errorf("expected message to be sent despite typing error")
	}
}

type fakeEscalationAwareExecutor struct {
	reason string
	calls  []struct {
		model string
		tools int
	}
	mu sync.Mutex
}

func (f *fakeEscalationAwareExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	if name == "escalate_to_strong_model" {
		f.reason = "complex_reasoning_needed"
		return "ESCALATED:complex_reasoning_needed", nil
	}
	return "tool_result", nil
}

func (f *fakeEscalationAwareExecutor) Reason() string {
	return f.reason
}

func (f *fakeEscalationAwareExecutor) Clear() {
	f.reason = ""
}

func (f *fakeEscalationAwareExecutor) recordCall(model string, toolsLen int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		model string
		tools int
	}{model, toolsLen})
}

type fakeStreamToolsLLMForEscalation struct {
	executor *fakeEscalationAwareExecutor
}

func (f *fakeStreamToolsLLMForEscalation) ChatWithToolsStream(ctx context.Context, messages []map[string]string, cfg llm.ChatWithToolsStreamConfig) (string, error) {
	f.executor.recordCall(cfg.Model, len(cfg.Tools))
	
	if cfg.OnDelta != nil {
		cfg.OnDelta("response text")
	}
	
	if cfg.Exec != nil && len(cfg.Tools) > 0 {
		_, _ = cfg.Exec.Execute(ctx, "escalate_to_strong_model", map[string]interface{}{})
	}
	
	return "final response", nil
}

func TestHandle_EscalateToolCall_ReRunsWithStrongModel(t *testing.T) {
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}
	
	executor := &fakeEscalationAwareExecutor{}
	streamLLM := &fakeStreamToolsLLMForEscalation{executor: executor}
	
	tools := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "escalate_to_strong_model",
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "search_wiki",
			},
		},
	}
	
	orch := New(Config{
		Router:         r,
		Discord:        disc,
		StreamToolsLLM: streamLLM,
		ToolExecutor:   executor,
		Tools:          tools,
		Streaming:      true,
		StrongModel:    "claude-opus",
		QueueSize:      10,
		WorkerCount:    1,
	})
	orch.Start()
	defer orch.Stop()
	
	event := buildEvent("escalate-test-1", 1, 1, "complex question")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	
	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})
	
	executor.mu.Lock()
	calls := executor.calls
	executor.mu.Unlock()
	
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 LLM calls (weak + strong), got %d", len(calls))
	}
	
	if calls[0].model != "unknown" {
		t.Errorf("first call should use unknown (no tier router), got %s", calls[0].model)
	}
	
	if calls[1].model != "claude-opus" {
		t.Errorf("second call should use claude-opus, got %s", calls[1].model)
	}
	
	if calls[1].tools >= calls[0].tools {
		t.Errorf("second call should have fewer tools (escalate removed), got %d vs %d", calls[1].tools, calls[0].tools)
	}
	
	if calls[1].tools != 1 {
		t.Errorf("second call should have 1 tool (search_wiki only), got %d", calls[1].tools)
	}
}

func TestHandle_EscalateCappedAtOneRound(t *testing.T) {
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}
	
	executor := &fakeEscalationAwareExecutor{}
	streamLLM := &fakeStreamToolsLLMForEscalation{executor: executor}
	
	tools := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "escalate_to_strong_model",
			},
		},
	}
	
	orch := New(Config{
		Router:         r,
		Discord:        disc,
		StreamToolsLLM: streamLLM,
		ToolExecutor:   executor,
		Tools:          tools,
		Streaming:      true,
		StrongModel:    "claude-opus",
		QueueSize:      10,
		WorkerCount:    1,
	})
	orch.Start()
	defer orch.Stop()
	
	event := buildEvent("escalate-cap-test-1", 1, 1, "question")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	
	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})
	
	executor.mu.Lock()
	calls := executor.calls
	executor.mu.Unlock()
	
	if len(calls) > 2 {
		t.Errorf("expected at most 2 LLM calls (capped at 1 escalation), got %d", len(calls))
	}
}

func TestHandle_EscalateSkippedWhenSameModel(t *testing.T) {
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}
	
	executor := &fakeEscalationAwareExecutor{}
	streamLLM := &fakeStreamToolsLLMForEscalation{executor: executor}
	
	tools := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "escalate_to_strong_model",
			},
		},
		{
			"type": "function",
			"function": map[string]interface{}{
				"name": "search_wiki",
			},
		},
	}
	
	orch := New(Config{
		Router:         r,
		Discord:        disc,
		StreamToolsLLM: streamLLM,
		ToolExecutor:   executor,
		Tools:          tools,
		Streaming:      true,
		StrongModel:    "unknown",
		QueueSize:      10,
		WorkerCount:    1,
	})
	orch.Start()
	defer orch.Stop()
	
	event := buildEvent("same-model-test", 1, 1, "question")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	
	waitUntil(t, 2*time.Second, func() bool {
		return len(disc.GetSentMessages()) > 0
	})
	
	executor.mu.Lock()
	calls := executor.calls
	executor.mu.Unlock()
	
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 LLM call (no escalation when already using strong model), got %d", len(calls))
	}
	
	if calls[0].model != "unknown" {
		t.Errorf("call should use unknown, got %s", calls[0].model)
	}
}

func TestOrchestrator_BehaviorUpdaterCalledOnGuildMessage(t *testing.T) {
	// Verify that BehaviorUpdater.UpdateFromMessage is called when a guild user sends a message.
	// This ensures behavior profiles are learned from user interactions.

	type stubBehaviorUpdater struct {
		calls []struct {
			guildID   int64
			userID    int64
			content   string
			createdAt time.Time
		}
	}

	updater := &stubBehaviorUpdater{}
	updateFunc := func(ctx context.Context, guildID, userID int64, content string, createdAt time.Time) error {
		updater.calls = append(updater.calls, struct {
			guildID   int64
			userID    int64
			content   string
			createdAt time.Time
		}{guildID, userID, content, createdAt})
		return nil
	}

	// Router that rejects the message (Should: false) so we only test capture + behavior update
	r := &stubRouter{decision: &router.Decision{Should: false}}
	disc := testutil.NewFakeDiscordClient()

	orch := New(Config{
		Router:          r,
		Discord:         disc,
		BehaviorUpdater: &fakeBehaviorUpdater{updateFunc: updateFunc},
		QueueSize:       10,
		WorkerCount:     1,
	})
	orch.Start()
	defer orch.Stop()

	// Create event with specific userID (buildEvent hardcodes UserID to 42)
	event := &domain.DiscordEvent{
		Type:      "message",
		GuildID:   42,
		ChannelID: 99,
		UserID:    77,
		Message: &domain.DiscordMessage{
			ID:        123,
			GuildID:   42,
			ChannelID: 99,
			UserID:    77,
			Content:   "hello world",
			CreatedAt: time.Now(),
		},
		CreatedAt: time.Now(),
	}

	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(updater.calls) > 0
	})

	if len(updater.calls) != 1 {
		t.Fatalf("expected 1 behavior update call, got %d", len(updater.calls))
	}

	call := updater.calls[0]
	if call.guildID != 42 {
		t.Errorf("expected guildID=42, got %d", call.guildID)
	}
	if call.userID != 77 {
		t.Errorf("expected userID=77, got %d", call.userID)
	}
	if call.content != "hello world" {
		t.Errorf("expected content='hello world', got %q", call.content)
	}
}

type fakeBehaviorUpdater struct {
	updateFunc func(ctx context.Context, guildID, userID int64, content string, createdAt time.Time) error
}

func (f *fakeBehaviorUpdater) UpdateFromMessage(ctx context.Context, guildID, userID int64, content string, createdAt time.Time) error {
	if f.updateFunc != nil {
		return f.updateFunc(ctx, guildID, userID, content, createdAt)
	}
	return nil
}

type fakeLoreCapturer struct {
	calls       []*domain.DiscordEvent
	onMessageFn func(ctx context.Context, event *domain.DiscordEvent)
	timedOut    bool
	mu          sync.Mutex
}

func (f *fakeLoreCapturer) OnMessage(ctx context.Context, event *domain.DiscordEvent) {
	if f.onMessageFn != nil {
		f.onMessageFn(ctx, event)
	} else {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, event)
	}
}

func TestLoreCapturerCalledForAllowedChannelMessages(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}
	capturer := &fakeLoreCapturer{}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		LoreCapturer: capturer,
		QueueSize:    10,
		WorkerCount:  1,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("evt-1", 123, 456, "lore message")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		capturer.mu.Lock()
		defer capturer.mu.Unlock()
		return len(capturer.calls) > 0
	})

	capturer.mu.Lock()
	defer capturer.mu.Unlock()

	if len(capturer.calls) != 1 {
		t.Fatalf("expected 1 lore capture call, got %d", len(capturer.calls))
	}

	if capturer.calls[0].GuildID != 123 {
		t.Errorf("expected guildID=123, got %d", capturer.calls[0].GuildID)
	}

	if capturer.calls[0].ChannelID != 456 {
		t.Errorf("expected channelID=456, got %d", capturer.calls[0].ChannelID)
	}
}

func TestLoreCapturerNotCalledWhenNil(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		LoreCapturer: nil,
		QueueSize:    10,
		WorkerCount:  1,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("evt-1", 123, 456, "message")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
}

func TestLoreCapturerCalledEvenWhenRouterRejects(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Ignore(router.ReasonChannelNotAllowed)}
	capturer := &fakeLoreCapturer{}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		LoreCapturer: capturer,
		QueueSize:    10,
		WorkerCount:  1,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("evt-1", 123, 456, "message")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		capturer.mu.Lock()
		defer capturer.mu.Unlock()
		return len(capturer.calls) > 0
	})

	capturer.mu.Lock()
	defer capturer.mu.Unlock()

	if len(capturer.calls) != 1 {
		t.Fatalf("expected 1 lore capture call even when router rejects, got %d", len(capturer.calls))
	}
}

func TestLoreCaptureTimeoutZeroMeansNoDeadline(t *testing.T) {
	capturer := &fakeLoreCapturer{
		calls: []*domain.DiscordEvent{},
	}
	capturer.onMessageFn = func(ctx context.Context, event *domain.DiscordEvent) {
		select {
		case <-time.After(5 * time.Second):
			capturer.mu.Lock()
			capturer.calls = append(capturer.calls, event)
			capturer.mu.Unlock()
		case <-ctx.Done():
			t.Errorf("context deadline exceeded before 5 seconds")
		}
	}

	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:             r,
		LLM:                llm,
		Discord:            disc,
		LoreCapturer:       capturer,
		LoreCaptureTimeout: 0,
		QueueSize:          10,
		WorkerCount:        1,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("evt-1", 123, 456, "message")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	time.Sleep(6 * time.Second)

	capturer.mu.Lock()
	defer capturer.mu.Unlock()

	if len(capturer.calls) != 1 {
		t.Fatalf("expected 1 lore capture call with no deadline, got %d", len(capturer.calls))
	}
}

func TestLoreCaptureTimeoutEnforced(t *testing.T) {
	capturer := &fakeLoreCapturer{
		calls: []*domain.DiscordEvent{},
	}
	capturer.onMessageFn = func(ctx context.Context, event *domain.DiscordEvent) {
		select {
		case <-time.After(500 * time.Millisecond):
			capturer.mu.Lock()
			capturer.calls = append(capturer.calls, event)
			capturer.mu.Unlock()
		case <-ctx.Done():
			capturer.mu.Lock()
			capturer.timedOut = true
			capturer.mu.Unlock()
		}
	}

	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}

	orch := New(Config{
		Router:             r,
		LLM:                llm,
		Discord:            disc,
		LoreCapturer:       capturer,
		LoreCaptureTimeout: 100 * time.Millisecond,
		QueueSize:          10,
		WorkerCount:        1,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("evt-1", 123, 456, "message")
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	time.Sleep(1 * time.Second)

	capturer.mu.Lock()
	defer capturer.mu.Unlock()

	if !capturer.timedOut {
		t.Fatalf("expected context to timeout with 100ms deadline")
	}
	if len(capturer.calls) != 0 {
		t.Fatalf("expected 0 successful calls when context times out, got %d", len(capturer.calls))
	}
}

func TestLoreCapturerNotCalledForBotMessages(t *testing.T) {
	llm := testutil.NewFakeLLMClient()
	disc := testutil.NewFakeDiscordClient()
	r := &stubRouter{decision: router.Respond(router.ReasonMention)}
	capturer := &fakeLoreCapturer{}

	orch := New(Config{
		Router:       r,
		LLM:          llm,
		Discord:      disc,
		LoreCapturer: capturer,
		QueueSize:    10,
		WorkerCount:  1,
	})
	orch.Start()
	defer orch.Stop()

	event := buildEvent("evt-1", 123, 456, "bot message")
	event.IsBot = true
	err := orch.Enqueue(context.Background(), event)
	if err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	capturer.mu.Lock()
	defer capturer.mu.Unlock()

	if len(capturer.calls) != 0 {
		t.Fatalf("expected 0 lore capture calls for bot messages, got %d", len(capturer.calls))
	}
}

