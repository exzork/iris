package safety

import (
	"strings"
	"testing"
)

func TestRedactSKKey(t *testing.T) {
	r := NewSecretRedactor()
	input := "Authorization: Bearer sk-abcdefghijklmnopqrstuvwxyz1234567890"
	output := r.Redact(input)
	if strings.Contains(output, "sk-") {
		t.Fatal("expected 'sk-' to be redacted")
	}
	if !strings.Contains(output, "[REDACTED_TOKEN]") {
		t.Fatal("expected '[REDACTED_TOKEN]' in output")
	}
}

func TestHasSensitiveDetectsSK(t *testing.T) {
	r := NewSecretRedactor()
	if !r.HasSensitive("sk-abcdefghijklmnopqrstuvwxyz1234567890") {
		t.Fatal("expected HasSensitive to detect sk- token")
	}
}

func TestRedactEmail(t *testing.T) {
	r := NewSecretRedactor()
	input := "contact me at user@example.com"
	output := r.Redact(input)
	if strings.Contains(output, "user@example.com") {
		t.Fatal("expected email to be redacted")
	}
}
