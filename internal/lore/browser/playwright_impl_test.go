package browser

import (
	"context"
	"testing"
)

func TestPlaywrightLookupUnavailableWhenExecMissing(t *testing.T) {
	pw := NewPlaywrightLookup("/nonexistent/path", 0)

	page, err := pw.Fetch(context.Background(), "https://example.com")
	if err != ErrBrowserUnavailable {
		t.Errorf("expected ErrBrowserUnavailable, got %v", err)
	}
	if page != nil {
		t.Errorf("expected nil page, got %v", page)
	}
}

func TestPlaywrightLookupClose(t *testing.T) {
	pw := NewPlaywrightLookup("/nonexistent/path", 0)
	err := pw.Close()
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
