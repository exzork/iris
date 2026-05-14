package slash

import (
	"encoding/json"
	"fmt"
	"strings"
)

// formatToolOutput turns a tool's raw stdout into a Discord-friendly
// message for direct slash invocations. Slash commands bypass the LLM, so
// without this step the user just sees raw JSON in chat. The formatter is
// keyed by tool name with a generic fallback that pretty-prints JSON or
// passes plain text through unchanged.
func formatToolOutput(toolName, raw string) string {
	out := strings.TrimSpace(raw)
	if out == "" {
		return "(tool kembali kosong)"
	}

	// Only attempt structured formatting on JSON payloads. Plain-text
	// tools (e.g. escalate_to_strong_model's status string) pass through.
	if !looksLikeJSON(out) {
		return out
	}

	switch toolName {
	case "web_search":
		if formatted, ok := formatWebSearch(out); ok {
			return formatted
		}
	case "mcp_list":
		if formatted, ok := formatMCPList(out); ok {
			return formatted
		}
	}

	// Generic JSON fallback: pretty-print inside a fenced block.
	return prettyJSONBlock(out)
}

func looksLikeJSON(s string) bool {
	if len(s) == 0 {
		return false
	}
	c := s[0]
	return c == '{' || c == '['
}

// webSearchEnvelope matches what websearch.Tool.Run emits:
//
//	{"provider":"<name>","results":[{"Title","URL","Snippet","Source","Authoritative"}]}
type webSearchEnvelope struct {
	Provider string `json:"provider"`
	Results  []struct {
		Title         string `json:"Title"`
		URL           string `json:"URL"`
		Snippet       string `json:"Snippet"`
		Source        string `json:"Source"`
		Authoritative bool   `json:"Authoritative"`
	} `json:"results"`
}

func formatWebSearch(raw string) (string, bool) {
	var env webSearchEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return "", false
	}
	if len(env.Results) == 0 {
		return "Gak ada hasil buat query itu.", true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**Hasil pencarian** (%d):\n", len(env.Results))
	for idx, r := range env.Results {
		title := strings.TrimSpace(r.Title)
		if title == "" {
			title = "(tanpa judul)"
		}
		fmt.Fprintf(&b, "\n**%d. [%s](%s)**", idx+1, title, r.URL)
		if r.Authoritative {
			b.WriteString(" `kanon`")
		}
		b.WriteString("\n")
		snippet := strings.TrimSpace(r.Snippet)
		if snippet != "" {
			if len([]rune(snippet)) > 240 {
				runes := []rune(snippet)
				snippet = string(runes[:240]) + "…"
			}
			b.WriteString(snippet)
			b.WriteString("\n")
		}
	}
	return b.String(), true
}

func formatMCPList(raw string) (string, bool) {
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return "", false
	}
	if len(names) == 0 {
		return "Belum ada MCP server yang terpasang.", true
	}
	var b strings.Builder
	b.WriteString("**MCP server terpasang:**\n")
	for _, n := range names {
		fmt.Fprintf(&b, "- `%s`\n", n)
	}
	return b.String(), true
}

func prettyJSONBlock(raw string) string {
	var tmp interface{}
	if err := json.Unmarshal([]byte(raw), &tmp); err != nil {
		return raw
	}
	pretty, err := json.MarshalIndent(tmp, "", "  ")
	if err != nil {
		return raw
	}
	return "```json\n" + string(pretty) + "\n```"
}
