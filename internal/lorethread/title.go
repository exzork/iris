package lorethread

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// LLMTitleGenerator generates titles for lore threads using an LLM.
type LLMTitleGenerator struct {
	llm     LLMCaller
	clock   Clock
	timeout time.Duration
}

// NewLLMTitleGenerator creates a new LLMTitleGenerator.
func NewLLMTitleGenerator(llm LLMCaller, clock Clock, timeout time.Duration) *LLMTitleGenerator {
	return &LLMTitleGenerator{
		llm:     llm,
		clock:   clock,
		timeout: timeout,
	}
}

// Generate produces a short Bahasa Indonesia title for a lore thread.
// On validation failure or LLM error, returns fallback title: "Ringkasan Lore — YYYY-MM-DD".
// If timeout is 0, uses ctx directly without adding a deadline.
func (g *LLMTitleGenerator) Generate(ctx context.Context, guildID int64, messages []*Message) (string, error) {
	if g.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.timeout)
		defer cancel()
	}

	systemPrompt := `You are a title generator for lore discussions. Create a short, descriptive Bahasa Indonesia title for the lore thread.

The title should be concise and capture the essence of the discussion.
Respond with only the title text, no quotes or additional commentary.`

	userPrompt := fmt.Sprintf(`Generate a short Bahasa Indonesia title for a lore discussion with %d messages.`, len(messages))

	response, err := g.llm.Call(ctx, systemPrompt, userPrompt)
	if err != nil {
		return g.fallbackTitle(), nil
	}

	title := strings.TrimSpace(response)

	// Validate the title
	if !g.isValidTitle(title) {
		return g.fallbackTitle(), nil
	}

	return title, nil
}

// isValidTitle checks if a title meets validation criteria.
func (g *LLMTitleGenerator) isValidTitle(title string) bool {
	// Reject empty or whitespace-only
	if len(strings.TrimSpace(title)) == 0 {
		return false
	}

	// Reject if longer than 80 characters
	if len(title) > 80 {
		return false
	}

	// Reject if contains control characters
	for _, r := range title {
		if unicode.IsControl(r) {
			return false
		}
	}

	// Reject if contains directive artifacts
	lowerTitle := strings.ToLower(title)
	forbiddenPatterns := []string{
		"system:",
		"assistant:",
		"user:",
		"ignore previous",
		"ignore prior",
	}
	for _, pattern := range forbiddenPatterns {
		if strings.Contains(lowerTitle, pattern) {
			return false
		}
	}

	return true
}

// fallbackTitle returns the fallback title with current date.
func (g *LLMTitleGenerator) fallbackTitle() string {
	return fmt.Sprintf("Ringkasan Lore — %s", g.clock.Now().UTC().Format("2006-01-02"))
}
