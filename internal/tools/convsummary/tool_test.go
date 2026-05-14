package convsummary

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestToolSchemaContract(t *testing.T) {
	tool := &Tool{S: &Summarizer{}}
	schema := tool.Schema()

	if schema.Name != "conversation_summarizer" {
		t.Errorf("expected name 'conversation_summarizer', got %q", schema.Name)
	}

	if schema.Description == "" {
		t.Errorf("expected non-empty description")
	}

	if len(schema.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(schema.Fields))
	}

	if err := schema.Validate(); err != nil {
		t.Errorf("schema validation failed: %v", err)
	}
}

func TestToolRunReturnsJSON(t *testing.T) {
	history := NewInMemoryHistory()
	now := time.Now()

	history.Add(100, 200, Message{
		UserID:    1,
		Username:  "alice",
		Content:   "hello",
		CreatedAt: now,
	})

	fakeLLM := &fakeLLM{result: "- Summary point"}
	redactor := NewRedactor()

	summarizer := &Summarizer{
		History: history,
		Redact:  redactor,
		LLM:     fakeLLM,
	}

	tool := &Tool{S: summarizer}

	result, err := tool.Run(context.Background(), map[string]interface{}{
		"guild_id":   float64(100),
		"channel_id": float64(200),
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if empty, ok := output["empty"].(bool); !ok || empty {
		t.Errorf("expected empty=false in output")
	}

	if _, ok := output["text"].(string); !ok {
		t.Errorf("expected text field in output")
	}

	if _, ok := output["channel_id"].(float64); !ok {
		t.Errorf("expected channel_id field in output")
	}
}

func TestToolRunMissingGuildError(t *testing.T) {
	tool := &Tool{S: &Summarizer{}}

	_, err := tool.Run(context.Background(), map[string]interface{}{
		"channel_id": float64(200),
	})

	if err == nil {
		t.Errorf("expected error for missing guild_id")
	}
}
