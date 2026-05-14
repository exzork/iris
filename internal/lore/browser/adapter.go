// Package browser provides headless browser-assisted lore lookup with compliance
// gates, rate limiting, and graceful fallback when browser is unavailable.
package browser

import (
	"context"
	"errors"
	"net/url"
	"time"
)

// RenderedPage represents a page fetched and rendered by a headless browser.
type RenderedPage struct {
	URL       string    // The URL that was fetched
	Title     string    // Extracted page title
	Text      string    // Extracted visible text content
	FetchedAt time.Time // When the page was fetched
}

// Errors returned by browser lookup operations.
var (
	ErrBrowserUnavailable = errors.New("browser unavailable")
	ErrHostNotRegistered  = errors.New("host not registered in source registry")
	ErrMethodNotAllowed   = errors.New("browser method not allowed by source policy")
	ErrRateLimitExceeded  = errors.New("rate limit exceeded")
	ErrNavigationFailed   = errors.New("browser navigation failed")
)

// BrowserLookup defines the interface for fetching and rendering pages.
type BrowserLookup interface {
	// Fetch renders a URL and returns title/text/URL. Respects ctx cancellation.
	Fetch(ctx context.Context, url string) (*RenderedPage, error)
	// Close releases browser resources.
	Close() error
}

// Lookup combines gate + limiter + browser for compliant, rate-limited lookups.
type Lookup struct {
	Gate    *Gate
	Limiter *Limiter
	Browser BrowserLookup
}

// Fetch performs a compliant, rate-limited browser lookup.
// It checks the compliance gate, enforces rate limits, and delegates to the browser.
// If the browser is unavailable, it returns ErrBrowserUnavailable.
func (l *Lookup) Fetch(ctx context.Context, rawURL string) (*RenderedPage, error) {
	// 1. Check compliance gate
	if err := l.Gate.Allow(rawURL); err != nil {
		return nil, err
	}

	// 2. Extract host and check rate limit
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if !l.Limiter.Allow(host) {
		return nil, ErrRateLimitExceeded
	}

	// 3. Fetch via browser
	page, err := l.Browser.Fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	return page, nil
}
