package slash

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSynth struct {
	reply      string
	err        error
	lastGuild  int64
	lastTool   string
	lastQuery  string
	lastOutput string
	calls      int
}

func (f *fakeSynth) Synthesize(ctx context.Context, guildID int64, toolName, userQuery, toolOutput string) (string, error) {
	f.calls++
	f.lastGuild = guildID
	f.lastTool = toolName
	f.lastQuery = userQuery
	f.lastOutput = toolOutput
	return f.reply, f.err
}

func TestRenderToolOutput_UsesSynthesizerWhenSet(t *testing.T) {
	r := &Router{}
	synth := &fakeSynth{reply: "Patch 3.0 udah rilis Desember kemarin, bro."}
	r.SetSynthesizer(synth)

	raw := `{"provider":"searxng","results":[{"Title":"3.0","URL":"https://x","Snippet":"dec 2025","Source":"searxng","Authoritative":false}]}`
	got := r.renderToolOutput(context.Background(), 42, "web_search", map[string]interface{}{"query": "patch terbaru"}, raw)

	if synth.calls != 1 {
		t.Fatalf("synthesizer should be called once, got %d", synth.calls)
	}
	if got != synth.reply {
		t.Errorf("expected synthesized reply, got: %q", got)
	}
	if synth.lastGuild != 42 || synth.lastTool != "web_search" || synth.lastQuery != "patch terbaru" {
		t.Errorf("synthesizer got wrong context: guild=%d tool=%q query=%q", synth.lastGuild, synth.lastTool, synth.lastQuery)
	}
	if !strings.Contains(synth.lastOutput, "searxng") {
		t.Errorf("synthesizer did not receive raw tool output")
	}
}

func TestRenderToolOutput_FallsBackOnSynthesizerError(t *testing.T) {
	r := &Router{}
	r.SetSynthesizer(&fakeSynth{err: errors.New("llm boom")})

	raw := `["filesystem"]`
	got := r.renderToolOutput(context.Background(), 1, "mcp_list", nil, raw)

	if !strings.Contains(got, "filesystem") || !strings.Contains(got, "MCP server terpasang") {
		t.Errorf("expected formatter fallback output, got: %q", got)
	}
}

func TestRenderToolOutput_FallsBackOnEmptySynthesizerReply(t *testing.T) {
	r := &Router{}
	r.SetSynthesizer(&fakeSynth{reply: "   "})

	raw := `["filesystem"]`
	got := r.renderToolOutput(context.Background(), 1, "mcp_list", nil, raw)

	if !strings.Contains(got, "filesystem") {
		t.Errorf("expected formatter fallback on empty synth reply, got: %q", got)
	}
}

func TestRenderToolOutput_NoSynthesizerUsesFormatter(t *testing.T) {
	r := &Router{}
	raw := `["a","b"]`
	got := r.renderToolOutput(context.Background(), 1, "mcp_list", nil, raw)
	if !strings.Contains(got, "`a`") || !strings.Contains(got, "`b`") {
		t.Errorf("expected formatter output with no synth, got: %q", got)
	}
}

func TestRenderToolOutput_EmptyRawReturnsSentinel(t *testing.T) {
	r := &Router{}
	r.SetSynthesizer(&fakeSynth{reply: "should not be called"})
	got := r.renderToolOutput(context.Background(), 1, "web_search", nil, "  ")
	if !strings.Contains(got, "kosong") {
		t.Errorf("expected empty-output sentinel, got: %q", got)
	}
}

func TestQueryFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{"nil map", nil, ""},
		{"empty map", map[string]interface{}{}, ""},
		{"query wins", map[string]interface{}{"query": "abc", "other": "xyz"}, "abc"},
		{"fallback to first string", map[string]interface{}{"name": "filesystem"}, "filesystem"},
		{"blank query falls through", map[string]interface{}{"query": "  ", "name": "fs"}, "fs"},
		{"non-string values ignored", map[string]interface{}{"limit": 5, "flag": true}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := queryFromArgs(tc.args); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
