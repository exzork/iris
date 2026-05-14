package safety

import (
	"strings"
	"testing"
)

func TestApplyRedactsSecrets(t *testing.T) {
	o := NewOutputFilter()
	res := o.Apply("ini token sk-test-123456789012345678901234567890 mohon simpan")

	if !res.Redacted {
		t.Fatalf("expected Redacted=true")
	}
	if strings.Contains(res.Content, "sk-test-") {
		t.Fatalf("expected secret token removed, got %q", res.Content)
	}
}

func TestApplyTruncatesLong(t *testing.T) {
	o := NewOutputFilter()
	res := o.Apply(strings.Repeat("x", 3000))

	if !res.Truncated {
		t.Fatalf("expected Truncated=true")
	}
	if len(res.Content) != 2000 {
		t.Fatalf("expected output length == 2000, got %d", len(res.Content))
	}
}

func TestApplyBlocksEmpty(t *testing.T) {
	o := NewOutputFilter()
	res := o.Apply("sk-test-123456789012345678901234567890")

	if !res.Blocked && strings.TrimSpace(res.Content) != "" {
		t.Fatalf("expected blocked output or empty content, got blocked=%v content=%q", res.Blocked, res.Content)
	}
	if res.Blocked && res.Reason != "empty_after_redaction" {
		t.Fatalf("expected reason empty_after_redaction, got %q", res.Reason)
	}
}

func TestApplyPassesCleanResponse(t *testing.T) {
	o := NewOutputFilter()
	in := "Rover tetap protagonis utama di cerita ini."
	res := o.Apply(in)

	if res.Blocked {
		t.Fatalf("expected clean response not blocked")
	}
	if res.Redacted {
		t.Fatalf("expected clean response not redacted")
	}
	if res.Truncated {
		t.Fatalf("expected clean response not truncated")
	}
	if res.Content != in {
		t.Fatalf("expected content unchanged, got %q", res.Content)
	}
}
