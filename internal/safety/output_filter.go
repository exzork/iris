package safety

import "strings"

const defaultDiscordMaxChars = 2000

type OutputFilter struct {
	Redactor *SecretRedactor
	MaxChars int // default 2000 (Discord message limit)
}

type FilterResult struct {
	Content   string
	Redacted  bool
	Truncated bool
	Blocked   bool   // if true, do NOT post
	Reason    string // reason for block
}

func NewOutputFilter() *OutputFilter {
	return &OutputFilter{
		Redactor: NewSecretRedactor(),
		MaxChars: defaultDiscordMaxChars,
	}
}

// Apply is called on the LLM's final Indonesian response before sending to Discord.
func (o *OutputFilter) Apply(content string) FilterResult {
	if o == nil {
		fallback := NewOutputFilter()
		return fallback.Apply(content)
	}

	redactor := o.Redactor
	if redactor == nil {
		redactor = NewSecretRedactor()
	}

	maxChars := o.MaxChars
	if maxChars <= 0 {
		maxChars = defaultDiscordMaxChars
	}

	redacted := redactor.Redact(content)
	result := FilterResult{
		Content:  redacted,
		Redacted: redacted != content,
	}

	if isEmptyAfterRedaction(redacted) {
		result.Blocked = true
		result.Reason = "empty_after_redaction"
		result.Content = ""
		return result
	}

	runes := []rune(result.Content)
	if len(runes) > maxChars {
		result.Content = string(runes[:maxChars])
		result.Truncated = true
	}

	return result
}

func isEmptyAfterRedaction(content string) bool {
	stripped := strings.TrimSpace(content)
	if stripped == "" {
		return true
	}

	for _, mask := range []string{"[REDACTED_EMAIL]", "[REDACTED_TOKEN]", "[REDACTED_SECRET]"} {
		stripped = strings.ReplaceAll(stripped, mask, "")
	}

	return strings.TrimSpace(stripped) == ""
}
