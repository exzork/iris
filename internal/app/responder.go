package app

import (
	"fmt"
	"strings"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
	"github.com/eko/iris-bot/internal/safety"
)

type Responder struct {
	Safety      *safety.SafetyPipeline
	PersonaText string
}

func NewResponder(pipeline *safety.SafetyPipeline, persona string) *Responder {
	return &Responder{Safety: pipeline, PersonaText: persona}
}

func (r *Responder) BuildMessages(query string, memoryFacts []string, loreCtx *ragpkg.PromptContext) []map[string]string {
	msgs := []map[string]string{
		{"role": "system", "content": r.PersonaText},
	}
	if loreCtx != nil && loreCtx.HasSupport {
		for i, snippet := range loreCtx.Snippets {
			title := ""
			if i < len(loreCtx.Citations) {
				title = loreCtx.Citations[i].Title
			}
			body := fmt.Sprintf("[LORE] %s\n%s", title, snippet)
			msgs = append(msgs, map[string]string{"role": "system", "content": r.Safety.SanitizeRetrieved(body)})
		}
	}
	for _, fact := range memoryFacts {
		body := "[MEMORY] " + fact
		msgs = append(msgs, map[string]string{"role": "system", "content": r.Safety.SanitizeRetrieved(body)})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": r.Safety.SanitizeRetrieved(query)})
	return msgs
}

func (r *Responder) WithCitations(text string, cits []ragpkg.Citation) string {
	if len(cits) == 0 {
		return text
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(text, "\n"))
	b.WriteString("\n\nSumber:\n")
	seen := map[string]bool{}
	for _, c := range cits {
		if seen[c.URL] {
			continue
		}
		seen[c.URL] = true
		fmt.Fprintf(&b, "- %q - %s\n", c.Title, c.URL)
	}
	return strings.TrimRight(b.String(), "\n")
}
