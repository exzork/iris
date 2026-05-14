package patchnotes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/eko/iris-bot/internal/tools"
	websearchpkg "github.com/eko/iris-bot/internal/tools/websearch"
)

func TestToolSchemaContract(t *testing.T) {
	tool := &Tool{
		S: &Summarizer{
			Search:     &fakeSearchPort{},
			RAG:        &fakeRAGPort{},
			MaxBullets: 5,
		},
	}

	schema := tool.Schema()

	if schema.Name != "patch_summarizer" {
		t.Errorf("Schema.Name = %q, want %q", schema.Name, "patch_summarizer")
	}

	if schema.Description == "" {
		t.Error("Schema.Description is empty")
	}

	if len(schema.Fields) != 2 {
		t.Errorf("len(Schema.Fields) = %d, want 2", len(schema.Fields))
	}

	queryField := schema.Fields[0]
	if queryField.Name != "query" {
		t.Errorf("Fields[0].Name = %q, want %q", queryField.Name, "query")
	}
	if queryField.Kind != tools.KindString {
		t.Errorf("Fields[0].Kind = %v, want %v", queryField.Kind, tools.KindString)
	}
	if !queryField.Required {
		t.Error("Fields[0].Required should be true")
	}

	maxBulletsField := schema.Fields[1]
	if maxBulletsField.Name != "max_bullets" {
		t.Errorf("Fields[1].Name = %q, want %q", maxBulletsField.Name, "max_bullets")
	}
	if maxBulletsField.Kind != tools.KindNumber {
		t.Errorf("Fields[1].Kind = %v, want %v", maxBulletsField.Kind, tools.KindNumber)
	}
	if maxBulletsField.Required {
		t.Error("Fields[1].Required should be false")
	}

	err := schema.Validate()
	if err != nil {
		t.Errorf("Schema.Validate() failed: %v", err)
	}
}

func TestToolRunReturnsJSON(t *testing.T) {
	search := &fakeSearchPort{
		results: []websearchpkg.SearchResult{
			{
				Title:   "Patch 1.4",
				Snippet: "New content",
				URL:     "https://wutheringwaves.com/patch-1.4",
			},
		},
	}

	rag := &fakeRAGPort{chunks: nil}

	tool := &Tool{
		S: &Summarizer{
			Search:     search,
			RAG:        rag,
			MaxBullets: 5,
		},
	}

	args := map[string]interface{}{
		"query": "patch 1.4",
	}

	result, err := tool.Run(context.Background(), args)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var summary Summary
	err = json.Unmarshal([]byte(result), &summary)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if summary.Query != "patch 1.4" {
		t.Errorf("Query = %q, want %q", summary.Query, "patch 1.4")
	}

	if len(summary.Bullets) != 1 {
		t.Errorf("len(Bullets) = %d, want 1", len(summary.Bullets))
	}
}

func TestToolRunMissingQueryError(t *testing.T) {
	tool := &Tool{
		S: &Summarizer{
			Search:     &fakeSearchPort{},
			RAG:        &fakeRAGPort{},
			MaxBullets: 5,
		},
	}

	args := map[string]interface{}{}

	_, err := tool.Run(context.Background(), args)
	if err == nil {
		t.Error("Run should fail with missing query")
	}
}

func TestToolRunWithMaxBullets(t *testing.T) {
	search := &fakeSearchPort{
		results: []websearchpkg.SearchResult{
			{Title: "Result 1", Snippet: "Snippet 1", URL: "https://example.com/1"},
			{Title: "Result 2", Snippet: "Snippet 2", URL: "https://example.com/2"},
			{Title: "Result 3", Snippet: "Snippet 3", URL: "https://example.com/3"},
		},
	}

	rag := &fakeRAGPort{chunks: nil}

	tool := &Tool{
		S: &Summarizer{
			Search:     search,
			RAG:        rag,
			MaxBullets: 5,
		},
	}

	args := map[string]interface{}{
		"query":       "test",
		"max_bullets": 2.0,
	}

	result, err := tool.Run(context.Background(), args)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var summary Summary
	err = json.Unmarshal([]byte(result), &summary)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(summary.Bullets) != 2 {
		t.Errorf("len(Bullets) = %d, want 2", len(summary.Bullets))
	}
}
