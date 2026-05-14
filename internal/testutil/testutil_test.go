package testutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eko/iris-bot/internal/domain"
)

func TestFakeDiscordClientSendMessage(t *testing.T) {
	client := NewFakeDiscordClient()

	err := client.SendMessage(context.Background(), 123, 456, "hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	msgs := client.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Fatalf("expected content 'hello', got %q", msgs[0].Content)
	}
}

func TestFakeDiscordClientSimulateError(t *testing.T) {
	client := NewFakeDiscordClient()
	client.SimulateError = errors.New("connection failed")

	err := client.SendMessage(context.Background(), 123, 456, "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "connection failed" {
		t.Fatalf("expected 'connection failed', got %q", err.Error())
	}
}

func TestFakeDiscordClientSimulateLatency(t *testing.T) {
	client := NewFakeDiscordClient()
	client.SimulateLatency = 50 * time.Millisecond

	start := time.Now()
	err := client.SendMessage(context.Background(), 123, 456, "hello")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("expected latency >= 50ms, got %v", elapsed)
	}
}

func TestFakeDiscordClientContextTimeout(t *testing.T) {
	client := NewFakeDiscordClient()
	client.SimulateLatency = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := client.SendMessage(ctx, 123, 456, "hello")
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
}

func TestFakeLLMClientChat(t *testing.T) {
	client := NewFakeLLMClient()
	client.ChatResponses["hello"] = "world"

	resp, err := client.Chat(context.Background(), 123, []map[string]string{
		{"role": "user", "content": "hello"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != "world" {
		t.Fatalf("expected 'world', got %q", resp)
	}
}

func TestFakeLLMClientSimulate429(t *testing.T) {
	client := NewFakeLLMClient()
	client.Simulate429 = true

	_, err := client.Chat(context.Background(), 123, []map[string]string{})
	if err == nil {
		t.Fatal("expected 429 error, got nil")
	}
	if err.Error() != "429 Too Many Requests" {
		t.Fatalf("expected '429 Too Many Requests', got %q", err.Error())
	}
}

func TestFakeLLMClientMalformedResponse(t *testing.T) {
	client := NewFakeLLMClient()
	client.SimulateMalformed = true

	resp, err := client.Chat(context.Background(), 123, []map[string]string{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != `{"incomplete": "json"` {
		t.Fatalf("expected malformed JSON, got %q", resp)
	}
}

func TestFakeLLMClientCallTool(t *testing.T) {
	client := NewFakeLLMClient()
	client.ToolResponses["search"] = `{"results": ["a", "b"]}`

	resp, err := client.CallTool(context.Background(), 123, "search", map[string]interface{}{"q": "test"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp != `{"results": ["a", "b"]}` {
		t.Fatalf("expected tool response, got %q", resp)
	}

	log := client.GetCallLog()
	if len(log) != 1 {
		t.Fatalf("expected 1 call, got %d", len(log))
	}
	if log[0].ToolName != "search" {
		t.Fatalf("expected tool 'search', got %q", log[0].ToolName)
	}
}

func TestFakeEmbeddingClientEmbed(t *testing.T) {
	client := NewFakeEmbeddingClient()

	vec, err := client.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(vec) != 1536 {
		t.Fatalf("expected 1536 dimensions, got %d", len(vec))
	}

	log := client.GetCallLog()
	if len(log) != 1 || log[0] != "test text" {
		t.Fatalf("expected call log to contain 'test text'")
	}
}

func TestFakeEmbeddingClientSimulateError(t *testing.T) {
	client := NewFakeEmbeddingClient()
	client.SimulateError = errors.New("embedding failed")

	_, err := client.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFakeImageClientGenerate(t *testing.T) {
	client := NewFakeImageClient()

	url, err := client.Generate(context.Background(), "a cat")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if url == "" {
		t.Fatal("expected non-empty URL")
	}

	urls := client.GetGeneratedURLs()
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
}

func TestFakeImageClientSimulateFailure(t *testing.T) {
	client := NewFakeImageClient()
	client.SimulateError = errors.New("image generation failed")

	_, err := client.Generate(context.Background(), "a cat")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFakeWebSearchSearch(t *testing.T) {
	client := NewFakeWebSearch()
	client.Results["iris"] = []string{"https://example.com/iris1", "https://example.com/iris2"}

	results, err := client.Search(context.Background(), "iris")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	log := client.GetCallLog()
	if len(log) != 1 || log[0] != "iris" {
		t.Fatalf("expected call log to contain 'iris'")
	}
}

func TestFakeWebSearchSimulateError(t *testing.T) {
	client := NewFakeWebSearch()
	client.SimulateError = errors.New("search failed")

	_, err := client.Search(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFakeStoragePortMemory(t *testing.T) {
	storage := NewFakeStoragePort()

	embedding := make([]float32, 10)
	err := storage.SaveMemory(context.Background(), 123, "test memory", embedding)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	memories, err := storage.GetMemories(context.Background(), 123, 10)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
	if memories[0].Content != "test memory" {
		t.Fatalf("expected 'test memory', got %q", memories[0].Content)
	}
}

func TestFakeStoragePortToolResult(t *testing.T) {
	storage := NewFakeStoragePort()

	result := &domain.ToolResult{
		ID:       "tool-1",
		ToolName: "search",
		Output:   "found something",
	}
	err := storage.SaveToolResult(context.Background(), result)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	retrieved, err := storage.GetToolResult(context.Background(), "tool-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected tool result, got nil")
	}
	if retrieved.Output != "found something" {
		t.Fatalf("expected 'found something', got %q", retrieved.Output)
	}
}

func TestFakeStoragePortExceptionChannel(t *testing.T) {
	storage := NewFakeStoragePort()

	err := storage.AddExceptionChannel(context.Background(), 123, 456)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	isException, err := storage.IsExceptionChannel(context.Background(), 123, 456)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !isException {
		t.Fatal("expected channel to be exception channel")
	}

	isException, err = storage.IsExceptionChannel(context.Background(), 123, 789)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if isException {
		t.Fatal("expected channel to not be exception channel")
	}
}

func TestFakeStoragePortLoreCitation(t *testing.T) {
	storage := NewFakeStoragePort()

	citation := &domain.LoreCitation{
		GuildID: 123,
		Source:  "Wuthering Waves Wiki",
		Content: "I.R.I.S is an AI",
		URL:     "https://example.com/iris",
	}
	err := storage.SaveLoreCitation(context.Background(), citation)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	citations, err := storage.GetLoreCitations(context.Background(), 123, 10)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(citations) != 1 {
		t.Fatalf("expected 1 citation, got %d", len(citations))
	}
	if citations[0].Content != "I.R.I.S is an AI" {
		t.Fatalf("expected 'I.R.I.S is an AI', got %q", citations[0].Content)
	}
}
