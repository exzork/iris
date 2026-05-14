package lorethread

import "regexp"

// Redactor redacts sensitive information from text.
type Redactor interface {
	Redact(text string) string
}

// RedactionRule defines a pattern and replacement for redaction.
type RedactionRule struct {
	Pattern     *regexp.Regexp
	Replacement string
}

// DefaultRedactor implements Redactor with a set of common redaction rules.
type DefaultRedactor struct {
	rules []RedactionRule
}

// NewDefaultRedactor creates a DefaultRedactor with standard redaction rules.
func NewDefaultRedactor() *DefaultRedactor {
	return &DefaultRedactor{
		rules: []RedactionRule{
			{
				// Discord bot token: 24 alphanumeric . 6 alphanumeric/dash/underscore . 27+ alphanumeric/dash/underscore
				Pattern:     regexp.MustCompile(`[A-Za-z0-9]{24}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}`),
				Replacement: "[REDACTED]",
			},
			{
				// OpenAI-style API key: sk- followed by alphanumeric/dash (20+ total after sk-)
				Pattern:     regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
				Replacement: "[REDACTED]",
			},
			{
				// GitHub personal access token: ghp_ followed by 36 alphanumeric
				Pattern:     regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`),
				Replacement: "[REDACTED]",
			},
			{
				// Slack bot token: xoxb- followed by alphanumeric/dash
				Pattern:     regexp.MustCompile(`xoxb-[A-Za-z0-9_-]+`),
				Replacement: "[REDACTED]",
			},
			{
				// Email addresses
				Pattern:     regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
				Replacement: "[REDACTED]",
			},
		},
	}
}

// Redact applies all redaction rules to the text.
func (r *DefaultRedactor) Redact(text string) string {
	result := text
	for _, rule := range r.rules {
		result = rule.Pattern.ReplaceAllString(result, rule.Replacement)
	}
	return result
}

// AddRule appends a custom redaction rule to the redactor.
func (r *DefaultRedactor) AddRule(pattern *regexp.Regexp, replacement string) {
	r.rules = append(r.rules, RedactionRule{
		Pattern:     pattern,
		Replacement: replacement,
	})
}
