package browser

import (
	"context"
	"testing"
	"time"

	loresource "github.com/eko/iris-bot/internal/lore/source"
)

// fakeBrowser is a test implementation of BrowserLookup.
type fakeBrowser struct {
	pages   map[string]*RenderedPage
	failOn  map[string]error
	fetched []string
}

func (f *fakeBrowser) Fetch(ctx context.Context, url string) (*RenderedPage, error) {
	f.fetched = append(f.fetched, url)
	if err, ok := f.failOn[url]; ok {
		return nil, err
	}
	p, ok := f.pages[url]
	if !ok {
		return nil, ErrNavigationFailed
	}
	return p, nil
}

func (f *fakeBrowser) Close() error {
	return nil
}

func TestLookupFetchesRegisteredHost(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}
	limiter := NewLimiter(1*time.Hour, 10)
	fake := &fakeBrowser{
		pages: map[string]*RenderedPage{
			"https://wutheringwaves.fandom.com/wiki/Rover": {
				URL:       "https://wutheringwaves.fandom.com/wiki/Rover",
				Title:     "Rover - Wuthering Waves Wiki",
				Text:      "Rover is a 5-star Spectro Resonator...",
				FetchedAt: time.Now(),
			},
		},
		failOn: make(map[string]error),
	}
	lookup := &Lookup{Gate: gate, Limiter: limiter, Browser: fake}

	page, err := lookup.Fetch(context.Background(), "https://wutheringwaves.fandom.com/wiki/Rover")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page == nil {
		t.Fatal("expected page, got nil")
	}
	if page.Title != "Rover - Wuthering Waves Wiki" {
		t.Errorf("title mismatch: %q", page.Title)
	}
	if len(fake.fetched) != 1 {
		t.Errorf("expected 1 fetch, got %d", len(fake.fetched))
	}
}

func TestLookupRejectsUnregisteredHost(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}
	limiter := NewLimiter(1*time.Hour, 10)
	fake := &fakeBrowser{
		pages:  make(map[string]*RenderedPage),
		failOn: make(map[string]error),
	}
	lookup := &Lookup{Gate: gate, Limiter: limiter, Browser: fake}

	_, err := lookup.Fetch(context.Background(), "https://example.com/foo")
	if err != ErrHostNotRegistered {
		t.Errorf("expected ErrHostNotRegistered, got %v", err)
	}
	if len(fake.fetched) != 0 {
		t.Errorf("expected 0 fetches, got %d", len(fake.fetched))
	}
}

func TestLookupRespectsRateLimit(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}
	limiter := NewLimiter(1*time.Hour, 1)
	fake := &fakeBrowser{
		pages: map[string]*RenderedPage{
			"https://wutheringwaves.fandom.com/wiki/Rover": {
				URL:       "https://wutheringwaves.fandom.com/wiki/Rover",
				Title:     "Rover",
				Text:      "...",
				FetchedAt: time.Now(),
			},
		},
		failOn: make(map[string]error),
	}
	lookup := &Lookup{Gate: gate, Limiter: limiter, Browser: fake}

	_, err := lookup.Fetch(context.Background(), "https://wutheringwaves.fandom.com/wiki/Rover")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}

	_, err = lookup.Fetch(context.Background(), "https://wutheringwaves.fandom.com/wiki/Rover")
	if err != ErrRateLimitExceeded {
		t.Errorf("expected ErrRateLimitExceeded, got %v", err)
	}
}

func TestLookupPropagatesBrowserUnavailable(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}
	limiter := NewLimiter(1*time.Hour, 10)
	fake := &fakeBrowser{
		pages: make(map[string]*RenderedPage),
		failOn: map[string]error{
			"https://wutheringwaves.fandom.com/wiki/Rover": ErrBrowserUnavailable,
		},
	}
	lookup := &Lookup{Gate: gate, Limiter: limiter, Browser: fake}

	_, err := lookup.Fetch(context.Background(), "https://wutheringwaves.fandom.com/wiki/Rover")
	if err != ErrBrowserUnavailable {
		t.Errorf("expected ErrBrowserUnavailable, got %v", err)
	}
}

func TestBrowserLookupInterface(t *testing.T) {
	var _ BrowserLookup = (*fakeBrowser)(nil)
}

func TestGateAllowFandom(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}

	err := gate.Allow("https://wutheringwaves.fandom.com/wiki/Rover")
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestGateRejectUnregistered(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}

	err := gate.Allow("https://example.com/foo")
	if err != ErrHostNotRegistered {
		t.Errorf("expected ErrHostNotRegistered, got %v", err)
	}
}

func TestGateRejectMethodNotAllowed(t *testing.T) {
	registry := loresource.NewRegistry()
	if err := registry.Register(&loresource.Source{
		ID:   "test_source",
		Host: "test.invalid",
		Policy: loresource.Policy{
			Name:           "Test Source",
			License:        "CC BY-SA 3.0",
			AttributionURL: "https://test.invalid",
			UserAgent:      "TestBot/1.0",
			RateLimitRPS:   1.0,
			AllowedMethods: []loresource.AccessMethod{loresource.MethodMediaWikiAPI},
			RequiresAttribution: true,
			NotesURL:       "https://test.invalid/tos",
		},
	}); err != nil {
		t.Fatalf("failed to register source: %v", err)
	}

	gate := &Gate{Registry: registry}
	err := gate.Allow("https://test.invalid/page")
	if err != ErrMethodNotAllowed {
		t.Errorf("expected ErrMethodNotAllowed, got %v", err)
	}
}

func TestGateBadURL(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}

	err := gate.Allow("ht!tp://[invalid")
	if err == nil {
		t.Error("expected error for bad URL")
	}
}
