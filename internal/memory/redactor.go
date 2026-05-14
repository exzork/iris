package memory

import (
	"regexp"
	"strings"
)

// Redactor rewrites potentially sensitive substrings before storage.
type Redactor struct {
	patterns []redactRule
}

type redactRule struct {
	name string
	re   *regexp.Regexp
	mask string
}

// NewRedactor returns a Redactor with the default rule set.
func NewRedactor() *Redactor {
	return &Redactor{
		patterns: []redactRule{
			// Emails.
			{
				name: "email",
				re:   regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`),
				mask: "[REDACTED_EMAIL]",
			},
			// Discord bot token style: three base64 chunks separated by dots.
			{
				name: "discord_token",
				re:   regexp.MustCompile(`[A-Za-z0-9_\-]{20,}\.[A-Za-z0-9_\-]{6,}\.[A-Za-z0-9_\-]{20,}`),
				mask: "[REDACTED_TOKEN]",
			},
			// OpenAI-style keys.
			{
				name: "openai_key",
				re:   regexp.MustCompile(`sk-[A-Za-z0-9_\-]{20,}`),
				mask: "[REDACTED_TOKEN]",
			},
			// Generic bearer tokens.
			{
				name: "bearer",
				re:   regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{10,}`),
				mask: "[REDACTED_TOKEN]",
			},
			// password=value / password: value (stop at whitespace or quote).
			{
				name: "password_kv",
				re:   regexp.MustCompile(`(?i)(password|passwd|pwd|secret|api[_\-]?key|token)\s*[:=]\s*["']?[^\s"']{4,}`),
				mask: "[REDACTED_SECRET]",
			},
			// AWS access key.
			{
				name: "aws_key",
				re:   regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
				mask: "[REDACTED_TOKEN]",
			},
			// Long hex strings that look like secrets (32+ hex chars).
			{
				name: "hex_secret",
				re:   regexp.MustCompile(`\b[a-fA-F0-9]{32,}\b`),
				mask: "[REDACTED_TOKEN]",
			},
		},
	}
}

// Redact applies all rules and returns the sanitized text.
func (r *Redactor) Redact(text string) string {
	out := text
	for _, rule := range r.patterns {
		out = rule.re.ReplaceAllString(out, rule.mask)
	}
	return out
}

// HasSensitive reports whether text matches any sensitive pattern.
func (r *Redactor) HasSensitive(text string) bool {
	for _, rule := range r.patterns {
		if rule.re.MatchString(text) {
			return true
		}
	}
	return false
}

// IsFullyRedacted reports whether the text after redaction is empty or mostly mask tokens.
func (r *Redactor) IsFullyRedacted(original string) bool {
	redacted := r.Redact(original)
	// If everything got collapsed into redaction masks, there's nothing worth storing.
	stripped := strings.TrimSpace(redacted)
	if stripped == "" {
		return true
	}
	// Strip known masks and see if anything meaningful remains.
	for _, mask := range []string{"[REDACTED_EMAIL]", "[REDACTED_TOKEN]", "[REDACTED_SECRET]"} {
		stripped = strings.ReplaceAll(stripped, mask, "")
	}
	stripped = strings.TrimSpace(stripped)
	return len(stripped) < 8
}
