package browser

import (
	"context"
	"os"
	"time"
)

// PlaywrightLookup is a headless browser adapter backed by Playwright.
// If the browser executable is unavailable, Fetch returns ErrBrowserUnavailable.
type PlaywrightLookup struct {
	ExecPath string
	Timeout  time.Duration
	checkFn  func(string) bool
}

// NewPlaywrightLookup creates a Playwright adapter with the given executable path and timeout.
func NewPlaywrightLookup(execPath string, timeout time.Duration) *PlaywrightLookup {
	return &PlaywrightLookup{
		ExecPath: execPath,
		Timeout:  timeout,
		checkFn:  defaultCheckFn,
	}
}

func defaultCheckFn(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Fetch renders a URL and returns the rendered page.
// If the browser executable is unavailable, it returns ErrBrowserUnavailable.
// Real Playwright wiring is deferred to a future integration task.
func (p *PlaywrightLookup) Fetch(ctx context.Context, url string) (*RenderedPage, error) {
	if !p.checkFn(p.ExecPath) {
		return nil, ErrBrowserUnavailable
	}

	// TODO: Wire to playwright-go or camoufox via os/exec.
	// For now, this is a stub that returns ErrBrowserUnavailable.
	return nil, ErrBrowserUnavailable
}

// Close releases browser resources.
func (p *PlaywrightLookup) Close() error {
	return nil
}
