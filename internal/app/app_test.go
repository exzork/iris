package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
	"github.com/eko/iris-bot/internal/router"
	"github.com/eko/iris-bot/internal/safety"
)

type fakeTriggerRouter struct {
	decision *router.Decision
	err      error
}

func (f *fakeTriggerRouter) Decide(ctx context.Context, event *domain.DiscordEvent) (*router.Decision, error) {
	return f.decision, f.err
}

type fakeMemory struct {
	promptContext []string
	promptErr     error
	considerCalls []string
	considerErr   error
}

func (f *fakeMemory) AssemblePromptContext(ctx context.Context, guildID int64, query string) ([]string, error) {
	return f.promptContext, f.promptErr
}

func (f *fakeMemory) Consider(ctx context.Context, guildID, userID int64, text string) (bool, error) {
	f.considerCalls = append(f.considerCalls, text)
	return len(f.considerCalls) > 0, f.considerErr
}

type fakeLore struct {
	promptCtx *ragpkg.PromptContext
	unsupport *ragpkg.UnsupportedResponse
	err       error
}

func (f *fakeLore) Compose(ctx context.Context, query string) (*ragpkg.PromptContext, *ragpkg.UnsupportedResponse, error) {
	return f.promptCtx, f.unsupport, f.err
}

type fakeLLM struct {
	response string
	err      error
	lastModel string
	lastMessages []map[string]string
}

func (f *fakeLLM) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	return f.ChatWithModel(ctx, "", guildID, messages)
}

func (f *fakeLLM) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	f.lastModel = model
	f.lastMessages = messages
	return f.response, f.err
}

type fakeSender struct {
	calls []senderCall
}

type senderCall struct {
	guildID   int64
	channelID int64
	content   string
}

func (f *fakeSender) Send(ctx context.Context, guildID, channelID int64, content string) error {
	f.calls = append(f.calls, senderCall{guildID, channelID, content})
	return nil
}

func TestHandleLoreAnswerWithCitation(t *testing.T) {
	pipeline := safety.NewSafetyPipeline()
	persona := "Saya adalah IRIS, asisten lore Wuthering Waves."

	loreCtx := &ragpkg.PromptContext{
		HasSupport: true,
		Citations: []ragpkg.Citation{
			{Title: "Rover", URL: "https://wutheringwaves.fandom.com/wiki/Rover"},
		},
		Snippets: []string{"Rover adalah protagonis Wuthering Waves."},
	}

	sender := &fakeSender{}
	app := New(
		&fakeTriggerRouter{decision: router.Respond(router.ReasonMention)},
		&fakeMemory{promptContext: []string{}},
		&fakeLore{promptCtx: loreCtx},
		&fakeLLM{response: "Rover adalah protagonis Wuthering Waves."},
		NewImagePipeline(nil),
		sender,
		pipeline,
		persona,
		nil,
	)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "Siapa itu Rover?",
		},
	}

	resp, err := app.Handle(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Sent {
		t.Errorf("expected Sent=true, got false")
	}
	if resp.Citations != 1 {
		t.Errorf("expected Citations=1, got %d", resp.Citations)
	}

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 sender call, got %d", len(sender.calls))
	}

	content := sender.calls[0].content
	if !strings.Contains(content, "Rover") {
		t.Errorf("content should contain 'Rover', got: %s", content)
	}
	if !strings.Contains(content, "wutheringwaves.fandom.com/wiki/Rover") {
		t.Errorf("content should contain citation URL, got: %s", content)
	}
	if !strings.Contains(content, "Sumber:") {
		t.Errorf("content should contain 'Sumber:' footer, got: %s", content)
	}
}

func TestHandleImageFailureSuppressesPost(t *testing.T) {
	pipeline := safety.NewSafetyPipeline()
	persona := "Saya adalah IRIS."

	fakeGen := &fakeImageGen{err: errors.New("quota exceeded")}
	sender := &fakeSender{}
	app := New(
		&fakeTriggerRouter{decision: router.Respond(router.ReasonMention)},
		&fakeMemory{promptContext: []string{}},
		&fakeLore{promptCtx: nil, unsupport: nil},
		&fakeLLM{response: "Berikut deskripsi Rover."},
		NewImagePipeline(fakeGen),
		sender,
		pipeline,
		persona,
		nil,
	)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "buatkan gambar Rover",
		},
	}

	resp, err := app.Handle(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Sent {
		t.Errorf("expected Sent=true, got false")
	}

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 sender call, got %d", len(sender.calls))
	}

	content := sender.calls[0].content
	if strings.Contains(content, "quota exceeded") {
		t.Errorf("content should NOT contain error message 'quota exceeded', got: %s", content)
	}
	if !strings.Contains(content, "Maaf, gambar tidak dapat dibuat saat ini.") {
		t.Errorf("content should contain fallback message, got: %s", content)
	}
	if resp.ImageURL != "" {
		t.Errorf("expected ImageURL empty, got: %s", resp.ImageURL)
	}
}

func TestHandleIgnoresExceptionChannel(t *testing.T) {
	pipeline := safety.NewSafetyPipeline()
	sender := &fakeSender{}
	app := New(
		&fakeTriggerRouter{decision: router.Ignore(router.ReasonExceptionChannel)},
		&fakeMemory{},
		&fakeLore{},
		&fakeLLM{},
		nil,
		sender,
		pipeline,
		"",
		nil,
	)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "test",
		},
	}

	resp, err := app.Handle(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Sent {
		t.Errorf("expected Sent=false, got true")
	}
	if len(sender.calls) != 0 {
		t.Errorf("expected no sender calls, got %d", len(sender.calls))
	}
}

func TestHandleMemoryInjectedBelowPersona(t *testing.T) {
	pipeline := safety.NewSafetyPipeline()
	persona := "Saya adalah IRIS."
	responder := NewResponder(pipeline, persona)

	loreCtx := &ragpkg.PromptContext{
		HasSupport: true,
		Citations: []ragpkg.Citation{
			{Title: "Test", URL: "https://example.com"},
		},
		Snippets: []string{"Test snippet"},
	}
	memoryFacts := []string{"User menyukai Rover"}
	query := "Siapa itu Rover?"

	msgs := responder.BuildMessages(query, memoryFacts, loreCtx)

	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	if msgs[0]["role"] != "system" || msgs[0]["content"] != persona {
		t.Errorf("msgs[0] should be persona system message")
	}

	personaIdx := 0
	memoryIdx := -1
	for i, msg := range msgs {
		if strings.Contains(msg["content"], "[MEMORY]") {
			memoryIdx = i
			break
		}
	}

	if memoryIdx <= personaIdx {
		t.Errorf("memory should come after persona, but memory at %d, persona at %d", memoryIdx, personaIdx)
	}
}

func TestHandleUnsupportedLoreAddsCaveat(t *testing.T) {
	pipeline := safety.NewSafetyPipeline()
	persona := "Saya adalah IRIS."
	sender := &fakeSender{}

	unsupported := &ragpkg.UnsupportedResponse{
		Message: "Belum ada data yang terindeks untuk pertanyaan ini.",
	}

	app := New(
		&fakeTriggerRouter{decision: router.Respond(router.ReasonMention)},
		&fakeMemory{promptContext: []string{}},
		&fakeLore{promptCtx: nil, unsupport: unsupported},
		&fakeLLM{response: "Maaf, saya tidak memiliki informasi tentang itu."},
		nil,
		sender,
		pipeline,
		persona,
		nil,
	)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "Siapa itu karakter yang tidak ada?",
		},
	}

	_, err := app.Handle(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 sender call, got %d", len(sender.calls))
	}

	content := sender.calls[0].content
	if !strings.Contains(content, "Belum ada data") {
		t.Errorf("content should contain unsupported message, got: %s", content)
	}
}

func TestHandlePersistsMemoryConsideration(t *testing.T) {
	pipeline := safety.NewSafetyPipeline()
	persona := "Saya adalah IRIS."
	memory := &fakeMemory{promptContext: []string{}}
	sender := &fakeSender{}

	app := New(
		&fakeTriggerRouter{decision: router.Respond(router.ReasonMention)},
		memory,
		&fakeLore{promptCtx: nil},
		&fakeLLM{response: "Aku suka jawaban singkat."},
		nil,
		sender,
		pipeline,
		persona,
		nil,
	)

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "aku suka jawaban singkat",
		},
	}

	_, err := app.Handle(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(memory.considerCalls) != 1 {
		t.Fatalf("expected 1 Consider call, got %d", len(memory.considerCalls))
	}

	if memory.considerCalls[0] != "aku suka jawaban singkat" {
		t.Errorf("Consider should receive query text, got: %s", memory.considerCalls[0])
	}
}

func TestAppHandlesNilPortsViaNoop(t *testing.T) {
	pipeline := safety.NewSafetyPipeline()
	persona := "Saya adalah IRIS."
	sender := &fakeSender{}

	app := New(
		&fakeTriggerRouter{decision: router.Respond(router.ReasonMention)},
		nil, // nil memory
		nil, // nil lore
		&fakeLLM{response: "Test response"},
		NewImagePipeline(nil),
		sender,
		pipeline,
		persona,
		nil, // nil logger
	)

	if app.Memory == nil {
		t.Fatal("expected Memory to be initialized with noop, got nil")
	}

	if app.Lore == nil {
		t.Fatal("expected Lore to be initialized with noop, got nil")
	}

	if app.Logger == nil {
		t.Fatal("expected Logger to be initialized with default, got nil")
	}

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "test query",
		},
	}

	resp, err := app.Handle(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Sent {
		t.Errorf("expected Sent=true, got false")
	}

	if len(sender.calls) != 1 {
		t.Fatalf("expected 1 sender call, got %d", len(sender.calls))
	}
}

func TestHandleUsesTierRouterModel(t *testing.T) {
	pipeline := safety.NewSafetyPipeline()
	persona := "Saya adalah IRIS, asisten lore Wuthering Waves."

	sender := &fakeSender{}
	llmClient := &fakeLLM{response: "Test response"}

	tierRouter := &llm.TierRouter{
		Classifier: &fakeClassifier{tier: llm.TierStrong},
		Router:     "kr/claude-haiku-4.5",
		Default:    "kr/claude-sonnet-4.5",
		Strong:     "M_STRONG",
	}

	app := New(
		&fakeTriggerRouter{decision: router.Respond(router.ReasonMention)},
		&fakeMemory{promptContext: []string{}},
		&fakeLore{},
		llmClient,
		NewImagePipeline(nil),
		sender,
		pipeline,
		persona,
		nil,
	)
	app.TierRouter = tierRouter

	event := &domain.DiscordEvent{
		Type:      "message_mention",
		GuildID:   123,
		ChannelID: 456,
		UserID:    789,
		Message: &domain.DiscordMessage{
			Content: "test query",
		},
	}

	resp, err := app.Handle(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.Sent {
		t.Errorf("expected Sent=true, got false")
	}

	if llmClient.lastModel != "M_STRONG" {
		t.Errorf("expected model M_STRONG, got %s", llmClient.lastModel)
	}
}

type fakeClassifier struct {
	tier llm.Tier
}

func (fc *fakeClassifier) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	if fc.tier == llm.TierStrong {
		return "STRONG", nil
	}
	return "DEFAULT", nil
}

