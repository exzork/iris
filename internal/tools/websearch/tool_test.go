package websearch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eko/iris-bot/internal/tools"
)

func TestToolSchemaContract(t *testing.T) {
	tool := New(NewFakeProvider(nil, nil))
	schema := tool.Schema()

	if schema.Name != "web_search" {
		t.Errorf("schema.Name = %q, want %q", schema.Name, "web_search")
	}

	if schema.Description == "" {
		t.Errorf("schema.Description is empty, want non-empty")
	}

	if len(schema.Fields) != 2 {
		t.Fatalf("schema.Fields has %d fields, want 2", len(schema.Fields))
	}

	queryField := schema.Fields[0]
	if queryField.Name != "query" {
		t.Errorf("fields[0].Name = %q, want %q", queryField.Name, "query")
	}
	if queryField.Kind != tools.KindString {
		t.Errorf("fields[0].Kind = %q, want %q", queryField.Kind, tools.KindString)
	}
	if !queryField.Required {
		t.Errorf("fields[0].Required = false, want true")
	}

	limitField := schema.Fields[1]
	if limitField.Name != "limit" {
		t.Errorf("fields[1].Name = %q, want %q", limitField.Name, "limit")
	}
	if limitField.Kind != tools.KindNumber {
		t.Errorf("fields[1].Kind = %q, want %q", limitField.Kind, tools.KindNumber)
	}
	if limitField.Required {
		t.Errorf("fields[1].Required = true, want false")
	}
}

func TestToolRunCallsProviderAndFormatsJSON(t *testing.T) {
	results := []SearchResult{
		{
			Title:         "Test Result",
			URL:           "https://example.com",
			Snippet:       "Test snippet",
			Source:        "test",
			Authoritative: false,
		},
	}

	provider := NewFakeProvider(results, nil)
	tool := New(provider)

	output, err := tool.Run(context.Background(), map[string]interface{}{
		"query": "test query",
	})

	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}

	if parsed["provider"] != "fake" {
		t.Errorf("provider = %q, want %q", parsed["provider"], "fake")
	}

	resultsVal, ok := parsed["results"].([]interface{})
	if !ok {
		t.Fatalf("results is not an array")
	}

	if len(resultsVal) != 1 {
		t.Fatalf("results has %d items, want 1", len(resultsVal))
	}
}

func TestToolRunRejectsEmptyQuery(t *testing.T) {
	provider := NewFakeProvider(nil, nil)
	tool := New(provider)

	_, err := tool.Run(context.Background(), map[string]interface{}{
		"query": "   ",
	})

	if err != ErrEmptyQuery {
		t.Errorf("Run() error = %v, want %v", err, ErrEmptyQuery)
	}
}

func TestToolRunClampsLimit(t *testing.T) {
	results := []SearchResult{
		{Title: "1", URL: "https://example.com/1", Snippet: "s1", Source: "test"},
		{Title: "2", URL: "https://example.com/2", Snippet: "s2", Source: "test"},
		{Title: "3", URL: "https://example.com/3", Snippet: "s3", Source: "test"},
		{Title: "4", URL: "https://example.com/4", Snippet: "s4", Source: "test"},
		{Title: "5", URL: "https://example.com/5", Snippet: "s5", Source: "test"},
	}

	tests := []struct {
		name     string
		limit    interface{}
		expected int
	}{
		{"limit 100 clamped to 10", float64(100), 5},
		{"limit 0 defaults to 5", float64(0), 5},
		{"limit negative defaults to 5", float64(-1), 5},
		{"limit 3 respected", float64(3), 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewFakeProvider(results, nil)
			tool := New(provider)

			output, err := tool.Run(context.Background(), map[string]interface{}{
				"query": "test",
				"limit": tt.limit,
			})

			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}

			var parsed map[string]interface{}
			json.Unmarshal([]byte(output), &parsed)

			resultsVal := parsed["results"].([]interface{})
			if len(resultsVal) != tt.expected {
				t.Errorf("got %d results, want %d", len(resultsVal), tt.expected)
			}
		})
	}
}

func TestToolRunPropagatesTimeoutError(t *testing.T) {
	provider := NewFakeProvider(nil, ErrTimeout)
	tool := New(provider)

	_, err := tool.Run(context.Background(), map[string]interface{}{
		"query": "test",
	})

	if err != ErrTimeout {
		t.Errorf("Run() error = %v, want %v", err, ErrTimeout)
	}
}
