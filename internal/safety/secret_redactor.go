package safety

import "github.com/eko/iris-bot/internal/memory"

type SecretRedactor struct {
	inner *memory.Redactor
}

func NewSecretRedactor() *SecretRedactor { return &SecretRedactor{inner: memory.NewRedactor()} }

// Redact returns a copy of text with secrets replaced.
func (r *SecretRedactor) Redact(text string) string {
	if r == nil || r.inner == nil {
		return memory.NewRedactor().Redact(text)
	}
	return r.inner.Redact(text)
}

// HasSensitive reports whether text currently contains any secret-looking pattern.
func (r *SecretRedactor) HasSensitive(text string) bool {
	if r == nil || r.inner == nil {
		return memory.NewRedactor().HasSensitive(text)
	}
	return r.inner.HasSensitive(text)
}
