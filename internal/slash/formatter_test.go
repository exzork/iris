package slash

import (
	"strings"
	"testing"
)

func TestFormatToolOutput_Empty(t *testing.T) {
	got := formatToolOutput("web_search", "")
	if !strings.Contains(got, "kosong") {
		t.Errorf("expected empty-output sentinel, got: %q", got)
	}
}

func TestFormatToolOutput_WebSearch_RendersMarkdown(t *testing.T) {
	raw := `{"provider":"searxng","results":[` +
		`{"Title":"Wuthering Waves 3.0","URL":"https://example.com/3.0","Snippet":"Christmas update","Source":"searxng","Authoritative":false},` +
		`{"Title":"Version 2.6","URL":"https://example.com/2.6","Snippet":"Releases Aug 28","Source":"searxng","Authoritative":true}` +
		`]}`
	got := formatToolOutput("web_search", raw)

	if strings.Contains(got, `{"provider"`) || strings.Contains(got, `"Title"`) {
		t.Fatalf("formatter leaked raw JSON: %s", got)
	}
	for _, want := range []string{"Hasil pencarian", "Wuthering Waves 3.0", "Christmas update", "Version 2.6", "Releases Aug 28", "kanon"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestFormatToolOutput_WebSearch_EmptyResults(t *testing.T) {
	raw := `{"provider":"searxng","results":[]}`
	got := formatToolOutput("web_search", raw)
	if !strings.Contains(got, "Gak ada hasil") {
		t.Errorf("expected no-results message, got: %q", got)
	}
}

func TestFormatToolOutput_MCPList(t *testing.T) {
	got := formatToolOutput("mcp_list", `["filesystem","github"]`)
	for _, want := range []string{"MCP server terpasang", "filesystem", "github"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestFormatToolOutput_MCPList_Empty(t *testing.T) {
	got := formatToolOutput("mcp_list", `[]`)
	if !strings.Contains(got, "Belum ada") {
		t.Errorf("expected empty-list sentinel, got: %q", got)
	}
}

func TestFormatToolOutput_GenericJSONFallback(t *testing.T) {
	got := formatToolOutput("unknown_tool", `{"k":1}`)
	if !strings.Contains(got, "```json") || !strings.Contains(got, `"k": 1`) {
		t.Errorf("expected pretty-printed JSON block, got: %q", got)
	}
}

func TestFormatToolOutput_PlainText_PassesThrough(t *testing.T) {
	got := formatToolOutput("escalate_to_strong_model", "switched to strong model")
	if got != "switched to strong model" {
		t.Errorf("plain text should pass through, got: %q", got)
	}
}

func TestFormatToolOutput_MalformedJSON_PassesThrough(t *testing.T) {
	raw := `{"provider":"searxng","results":[{broken`
	got := formatToolOutput("web_search", raw)
	if got != raw {
		t.Errorf("malformed JSON should pass through unchanged, got: %q", got)
	}
}
