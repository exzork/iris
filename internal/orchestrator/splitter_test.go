package orchestrator

import (
	"strings"
	"testing"
)

func TestSplitMessageShort(t *testing.T) {
	got := SplitMessage("hello", 2000)
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got))
	}
	if got[0] != "hello" {
		t.Errorf("expected %q, got %q", "hello", got[0])
	}
}

func TestSplitMessageEmpty(t *testing.T) {
	got := SplitMessage("", 2000)
	if len(got) != 0 {
		t.Errorf("expected 0 chunks for empty, got %d", len(got))
	}
}

func TestSplitMessageExactLimit(t *testing.T) {
	content := strings.Repeat("a", 2000)
	got := SplitMessage(content, 2000)
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk at exact limit, got %d", len(got))
	}
	if len(got[0]) != 2000 {
		t.Errorf("expected length 2000, got %d", len(got[0]))
	}
}

func TestSplitMessageOverLimit(t *testing.T) {
	content := strings.Repeat("a", 2500)
	got := SplitMessage(content, 2000)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(got))
	}
	for i, chunk := range got {
		if len(chunk) > 2000 {
			t.Errorf("chunk %d exceeds 2000: got %d", i, len(chunk))
		}
	}
	if strings.Join(got, "") != content {
		t.Errorf("joined chunks should equal original content")
	}
}

func TestSplitMessagePrefersNewline(t *testing.T) {
	line := strings.Repeat("a", 500)
	content := line + "\n" + line + "\n" + line + "\n" + line + "\n" + line
	got := SplitMessage(content, 2000)
	if len(got) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(got))
	}
	for i, chunk := range got {
		if len(chunk) > 2000 {
			t.Errorf("chunk %d exceeds 2000 chars: got %d", i, len(chunk))
		}
	}
}

func TestSplitMessagePrefersSpace(t *testing.T) {
	words := []string{}
	for i := 0; i < 500; i++ {
		words = append(words, strings.Repeat("x", 10))
	}
	content := strings.Join(words, " ")
	got := SplitMessage(content, 2000)
	for i, chunk := range got {
		if len(chunk) > 2000 {
			t.Errorf("chunk %d exceeds 2000: got %d", i, len(chunk))
		}
	}
}

func TestSplitMessageVeryLongNoSeparator(t *testing.T) {
	content := strings.Repeat("a", 4500)
	got := SplitMessage(content, 2000)
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(got))
	}
	total := 0
	for _, c := range got {
		total += len(c)
	}
	if total != 4500 {
		t.Errorf("total length mismatch: %d", total)
	}
}

func TestSplitMessageExact2000CharBoundary(t *testing.T) {
	content := strings.Repeat("x", 2001)
	got := SplitMessage(content, 2000)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks for 2001 chars, got %d", len(got))
	}
	if len(got[0]) != 2000 {
		t.Errorf("first chunk should be exactly 2000, got %d", len(got[0]))
	}
	if len(got[1]) != 1 {
		t.Errorf("second chunk should be 1, got %d", len(got[1]))
	}
	if got[0]+got[1] != content {
		t.Errorf("chunks don't reconstruct original")
	}
}

func TestSplitMessage4500CharsWithNewlines(t *testing.T) {
	line := strings.Repeat("a", 1000)
	content := line + "\n" + line + "\n" + line + "\n" + line + "\n" + strings.Repeat("b", 500)
	got := SplitMessage(content, 2000)
	
	if len(got) < 2 {
		t.Fatalf("expected at least 2 chunks for 4500 chars with newlines, got %d", len(got))
	}
	
	for i, chunk := range got {
		if len(chunk) > 2000 {
			t.Errorf("chunk %d exceeds 2000: got %d", i, len(chunk))
		}
	}
	
	reconstructed := strings.Join(got, "")
	if reconstructed != content {
		t.Errorf("reconstructed content doesn't match original")
	}
}

func TestSplitMessageSingleLongWordHardSplit(t *testing.T) {
	longWord := strings.Repeat("supercalifragilisticexpialidocious", 100)
	content := longWord
	got := SplitMessage(content, 2000)
	
	if len(got) < 2 {
		t.Fatalf("expected at least 2 chunks for single long word, got %d", len(got))
	}
	
	for i, chunk := range got {
		if len(chunk) > 2000 {
			t.Errorf("chunk %d exceeds 2000: got %d", i, len(chunk))
		}
	}
	
	reconstructed := strings.Join(got, "")
	if reconstructed != content {
		t.Errorf("reconstructed content doesn't match original")
	}
}

func TestSplitMessageChunkOrdering(t *testing.T) {
	content := "first" + strings.Repeat("x", 2500) + "second" + strings.Repeat("y", 2500) + "third"
	got := SplitMessage(content, 2000)
	
	if len(got) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(got))
	}
	
	reconstructed := strings.Join(got, "")
	if reconstructed != content {
		t.Errorf("chunk ordering incorrect: reconstructed doesn't match original")
	}
	
	if !strings.HasPrefix(reconstructed, "first") {
		t.Errorf("first chunk should start with 'first'")
	}
	if !strings.HasSuffix(reconstructed, "third") {
		t.Errorf("last chunk should end with 'third'")
	}
}

func TestSplitMessageNoChunkExceedsLimit(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		limit   int
	}{
		{"4500 chars", strings.Repeat("a", 4500), 2000},
		{"10000 chars", strings.Repeat("b", 10000), 2000},
		{"mixed content", "hello\n" + strings.Repeat("x", 5000) + "\nworld", 2000},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := SplitMessage(tc.content, tc.limit)
			for i, chunk := range got {
				if len(chunk) > tc.limit {
					t.Errorf("chunk %d exceeds limit %d: got %d", i, tc.limit, len(chunk))
				}
			}
			if strings.Join(got, "") != tc.content {
				t.Errorf("reconstructed content doesn't match original")
			}
		})
	}
}
