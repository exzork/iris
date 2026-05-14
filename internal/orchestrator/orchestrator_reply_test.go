package orchestrator

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/router"
)

type recordedSend struct {
	kind             string
	guildID          int64
	channelID        int64
	replyToMessageID int64
	mentionRepliedUser bool
	content          string
}

type replySendRecorder struct {
	mu    sync.Mutex
	sends []recordedSend
}

func (r *replySendRecorder) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sends = append(r.sends, recordedSend{kind: "send", guildID: guildID, channelID: channelID, content: content})
	return nil
}

func (r *replySendRecorder) ReplyMessage(ctx context.Context, guildID, channelID, replyToMessageID int64, content string, mentionRepliedUser bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sends = append(r.sends, recordedSend{kind: "reply", guildID: guildID, channelID: channelID, replyToMessageID: replyToMessageID, mentionRepliedUser: mentionRepliedUser, content: content})
	return nil
}

func (r *replySendRecorder) snapshot() []recordedSend {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedSend, len(r.sends))
	copy(out, r.sends)
	return out
}

type replyTestLLM struct {
	response string
}

func (l *replyTestLLM) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	return l.response, nil
}

func (l *replyTestLLM) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	return l.response, nil
}

type replyTestDecider struct{}

func (replyTestDecider) Decide(ctx context.Context, event *domain.DiscordEvent) (*router.Decision, error) {
	return &router.Decision{Should: true, Reason: router.ReasonMention}, nil
}

func runOrchestratorWithSender(t *testing.T, sender interface {
	SendMessage(ctx context.Context, guildID, channelID int64, content string) error
}, response string, msgID int64) {
	t.Helper()

	cfg := Config{
		Router:       replyTestDecider{},
		LLM:          &replyTestLLM{response: response},
		Discord:      sender,
		ContextStore: &integrationTestContextStore{},
		SystemPrompt: "system",
		QueueSize:    8,
		WorkerCount:  1,
		JobTimeout:   5 * time.Second,
	}

	orch := New(cfg)
	orch.Start()
	defer orch.Stop()

	event := &domain.DiscordEvent{
		GuildID:   123,
		ChannelID: 456,
		UserID:    777,
		Message: &domain.DiscordMessage{
			ID:      msgID,
			Content: "trigger",
		},
	}

	if err := orch.Enqueue(context.Background(), event); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	time.Sleep(400 * time.Millisecond)
}

func TestOrchestrator_NonStreamingReply_FirstChunkPings(t *testing.T) {
	rec := &replySendRecorder{}
	runOrchestratorWithSender(t, rec, "short response", 999)

	sends := rec.snapshot()
	if len(sends) != 1 {
		t.Fatalf("expected exactly 1 send, got %d (%+v)", len(sends), sends)
	}
	got := sends[0]
	if got.kind != "reply" {
		t.Fatalf("expected reply kind, got %q", got.kind)
	}
	if got.replyToMessageID != 999 {
		t.Errorf("replyToMessageID = %d, want %d", got.replyToMessageID, 999)
	}
	if !got.mentionRepliedUser {
		t.Errorf("expected ping enabled (mentionRepliedUser=true)")
	}
}

func TestOrchestrator_NonStreamingReply_ChunkedRespondsOnceWithReplyThenPlain(t *testing.T) {
	rec := &replySendRecorder{}
	long := strings.Repeat("a", DiscordMessageLimit) + strings.Repeat("b", 200)
	runOrchestratorWithSender(t, rec, long, 1234)

	sends := rec.snapshot()
	if len(sends) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(sends))
	}
	if sends[0].kind != "reply" {
		t.Errorf("first chunk should be reply, got %q", sends[0].kind)
	}
	if !sends[0].mentionRepliedUser {
		t.Errorf("first chunk should ping the replied user")
	}
	for i := 1; i < len(sends); i++ {
		if sends[i].kind != "send" {
			t.Errorf("subsequent chunk %d should be plain send, got %q (would re-ping)", i, sends[i].kind)
		}
	}
}

type plainOnlySender struct {
	mu    sync.Mutex
	calls []string
}

func (p *plainOnlySender) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, content)
	return nil
}

func TestOrchestrator_NonStreamingReply_FallbackWhenSenderLacksReply(t *testing.T) {
	plain := &plainOnlySender{}
	runOrchestratorWithSender(t, plain, "fallback response", 42)

	plain.mu.Lock()
	defer plain.mu.Unlock()
	if len(plain.calls) != 1 {
		t.Fatalf("expected 1 plain send, got %d", len(plain.calls))
	}
	if plain.calls[0] != "fallback response" {
		t.Errorf("content = %q, want %q", plain.calls[0], "fallback response")
	}
}
