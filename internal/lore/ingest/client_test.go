package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(serverURL string, ua string) *HTTPMediaWikiClient {
	c := NewHTTPMediaWikiClient(serverURL, "https://example.invalid/wiki/", ua)
	c.MinInterval = 0
	return c
}

func TestListPagesParsesAPIResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "query" {
			t.Fatalf("expected action=query, got %q", r.URL.Query().Get("action"))
		}
		_, _ = w.Write([]byte(`{"query":{"allpages":[{"pageid":1,"title":"A"},{"pageid":2,"title":"B"}]}}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, "iris-bot-test")
	pages, err := client.ListPages(context.Background(), "", 2)
	if err != nil {
		t.Fatalf("ListPages() error = %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0].ID != 1 || pages[0].Title != "A" {
		t.Fatalf("unexpected first page: %+v", pages[0])
	}
	if pages[1].ID != 2 || pages[1].Title != "B" {
		t.Fatalf("unexpected second page: %+v", pages[1])
	}
}

func TestListPagesPassesApFromTitle(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Query().Get("apfrom")
		_, _ = w.Write([]byte(`{"query":{"allpages":[]}}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, "iris-bot-test")
	if _, err := client.ListPages(context.Background(), "Rover", 5); err != nil {
		t.Fatalf("ListPages() error = %v", err)
	}
	if seen != "Rover" {
		t.Fatalf("expected apfrom=Rover, got %q", seen)
	}
}

func TestGetPageParsesWikitext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "parse" {
			t.Fatalf("expected action=parse, got %q", r.URL.Query().Get("action"))
		}
		_, _ = w.Write([]byte(`{"parse":{"pageid":42,"title":"Lore Page","revid":77,"wikitext":{"*":"line one\n\nline two"}}}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, "iris-bot-test")
	page, err := client.GetPage(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetPage() error = %v", err)
	}
	if page.ID != 42 {
		t.Fatalf("expected ID 42, got %d", page.ID)
	}
	if page.Title != "Lore Page" {
		t.Fatalf("expected title Lore Page, got %q", page.Title)
	}
	if page.Revision != 77 {
		t.Fatalf("expected rev 77, got %d", page.Revision)
	}
	if page.Wikitext != "line one\n\nline two" {
		t.Fatalf("unexpected wikitext: %q", page.Wikitext)
	}
	if page.URL != "https://example.invalid/wiki/Lore_Page" {
		t.Fatalf("expected canonical wiki URL, got %q", page.URL)
	}
}

func TestRetryOn5xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"temporary"}`))
			return
		}
		_, _ = w.Write([]byte(`{"query":{"allpages":[{"pageid":3,"title":"Recovered"}]}}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, "iris-bot-test")
	client.RetryDelay = 5 * time.Millisecond
	client.MaxRetries = 2

	pages, err := client.ListPages(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("ListPages() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if len(pages) != 1 || pages[0].ID != 3 {
		t.Fatalf("unexpected pages: %+v", pages)
	}
}

func TestUserAgentSent(t *testing.T) {
	const ua = "iris-bot/1.0"
	seenUA := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"query":{"allpages":[]}}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, ua)
	_, err := client.ListPages(context.Background(), "", 1)
	if err != nil {
		t.Fatalf("ListPages() error = %v", err)
	}
	if seenUA != ua {
		t.Fatalf("expected user-agent %q, got %q", ua, seenUA)
	}
}

func TestMinIntervalEnforced(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"query":{"allpages":[]}}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, "iris-bot-test")
	client.MinInterval = 50 * time.Millisecond

	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := client.ListPages(context.Background(), "", 1); err != nil {
			t.Fatalf("ListPages() error = %v", err)
		}
	}
	elapsed := time.Since(start)
	if hits != 3 {
		t.Fatalf("expected 3 hits, got %d", hits)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("rate limiter did not space requests; elapsed=%v", elapsed)
	}
}
