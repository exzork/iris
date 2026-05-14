package lorethread

import (
	"regexp"
	"testing"
)

func TestRedactor_DiscordBotToken(t *testing.T) {
	redactor := NewDefaultRedactor()
	text := "My token is NzkyNDcyNzI0MTk2ODk2Nzk1.X-hvzA.Ovy4MCQywSkoMRRclStW4xAYK2c and it's secret"
	result := redactor.Redact(text)

	if contains(result, "NzkyNDcyNzI0MTk2ODk2Nzk1") {
		t.Errorf("expected Discord token to be redacted, got %q", result)
	}
	if !contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in result, got %q", result)
	}
}

func TestRedactor_OpenAIAPIKey(t *testing.T) {
	redactor := NewDefaultRedactor()
	text := "Use this key: sk-proj-1234567890123456789012345678901234567890 for API calls"
	result := redactor.Redact(text)

	if contains(result, "sk-proj-") {
		t.Errorf("expected OpenAI key to be redacted, got %q", result)
	}
	if !contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in result, got %q", result)
	}
}

func TestRedactor_GitHubToken(t *testing.T) {
	redactor := NewDefaultRedactor()
	text := "GitHub token: ghp_1234567890123456789012345678901234567890"
	result := redactor.Redact(text)

	if contains(result, "ghp_") {
		t.Errorf("expected GitHub token to be redacted, got %q", result)
	}
	if !contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in result, got %q", result)
	}
}

func TestRedactor_SlackBotToken(t *testing.T) {
	redactor := NewDefaultRedactor()
	text := "Slack bot: xoxb-1234567890-1234567890-abcdefghijklmnopqrst"
	result := redactor.Redact(text)

	if contains(result, "xoxb-") {
		t.Errorf("expected Slack token to be redacted, got %q", result)
	}
	if !contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in result, got %q", result)
	}
}

func TestRedactor_EmailAddress(t *testing.T) {
	redactor := NewDefaultRedactor()
	text := "Contact me at user@example.com for details"
	result := redactor.Redact(text)

	if contains(result, "user@example.com") {
		t.Errorf("expected email to be redacted, got %q", result)
	}
	if !contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in result, got %q", result)
	}
}

func TestRedactor_SafeStringUntouched(t *testing.T) {
	redactor := NewDefaultRedactor()
	text := "This is a safe message about lore and worldbuilding"
	result := redactor.Redact(text)

	if result != text {
		t.Errorf("expected safe string to be untouched, got %q", result)
	}
}

func TestRedactor_MultipleTokens(t *testing.T) {
	redactor := NewDefaultRedactor()
	text := "Token1: sk-1234567890123456789012 and Token2: sk-9876543210987654321098"
	result := redactor.Redact(text)

	if contains(result, "sk-") {
		t.Errorf("expected all tokens to be redacted, got %q", result)
	}

	redactedCount := 0
	for i := 0; i <= len(result)-len("[REDACTED]"); i++ {
		if result[i:i+len("[REDACTED]")] == "[REDACTED]" {
			redactedCount++
		}
	}
	if redactedCount < 2 {
		t.Errorf("expected at least 2 [REDACTED] markers, got %d", redactedCount)
	}
}

func TestRedactor_AddCustomRule(t *testing.T) {
	redactor := NewDefaultRedactor()
	customPattern := regexp.MustCompile(`SECRET:\s*\w+`)
	redactor.AddRule(customPattern, "[CUSTOM_REDACTED]")

	text := "Here is SECRET: mypassword in the text"
	result := redactor.Redact(text)

	if contains(result, "SECRET:") {
		t.Errorf("expected custom rule to redact, got %q", result)
	}
	if !contains(result, "[CUSTOM_REDACTED]") {
		t.Errorf("expected [CUSTOM_REDACTED] in result, got %q", result)
	}
}

func TestRedactor_ComplexEmail(t *testing.T) {
	redactor := NewDefaultRedactor()
	text := "Email: john.doe+tag@sub.example.co.uk is valid"
	result := redactor.Redact(text)

	if contains(result, "@") {
		t.Errorf("expected email to be redacted, got %q", result)
	}
	if !contains(result, "[REDACTED]") {
		t.Errorf("expected [REDACTED] in result, got %q", result)
	}
}
