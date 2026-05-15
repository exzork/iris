package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Page struct {
	ID        int64
	Title     string
	Wikitext  string
	URL       string
	Revision  int64
	UpdatedAt time.Time
}

type PageSummary struct {
	ID    int64
	Title string
}

type MediaWikiClient interface {
	ListPages(ctx context.Context, fromTitle string, limit int) ([]PageSummary, error)
	ListPagesAt(ctx context.Context, fromTitle string, apContinue string, limit int) (pages []PageSummary, nextContinue string, err error)
	GetPage(ctx context.Context, id int64) (*Page, error)
}

// HTTPMediaWikiClient hits the MediaWiki action API.
// Fandom example: APIBaseURL="https://wutheringwaves.fandom.com/api.php",
// PageBaseURL="https://wutheringwaves.fandom.com/wiki/".
// HTML scraping is intentionally not used.
type HTTPMediaWikiClient struct {
	APIBaseURL  string
	PageBaseURL string
	UserAgent   string
	HTTP        *http.Client
	MaxRetries  int
	RetryDelay  time.Duration
	MinInterval time.Duration

	lastRequest time.Time
}

func NewHTTPMediaWikiClient(apiBaseURL, pageBaseURL, userAgent string) *HTTPMediaWikiClient {
	return &HTTPMediaWikiClient{
		APIBaseURL:  strings.TrimSpace(apiBaseURL),
		PageBaseURL: strings.TrimSpace(pageBaseURL),
		UserAgent:   userAgent,
		HTTP:        &http.Client{Timeout: 20 * time.Second},
		MaxRetries:  2,
		RetryDelay:  100 * time.Millisecond,
		MinInterval: time.Second,
	}
}

func (c *HTTPMediaWikiClient) ListPages(ctx context.Context, fromTitle string, limit int) ([]PageSummary, error) {
	pages, _, err := c.ListPagesAt(ctx, fromTitle, "", limit)
	return pages, err
}

// ListPagesAt is the cursor-aware list operation. apContinue is the opaque
// token returned by a prior call; when set, fromTitle is ignored. The
// returned nextContinue should be persisted as the cursor for the next
// invocation. An empty nextContinue means the source is exhausted.
func (c *HTTPMediaWikiClient) ListPagesAt(ctx context.Context, fromTitle string, apContinue string, limit int) ([]PageSummary, string, error) {
	if strings.TrimSpace(c.APIBaseURL) == "" {
		return nil, "", errors.New("mediawiki client: APIBaseURL is required")
	}
	if limit <= 0 {
		limit = 1
	}

	requestLimit := limit
	if apContinue == "" && strings.TrimSpace(fromTitle) != "" {
		requestLimit = limit + 1
	}

	q := url.Values{}
	q.Set("action", "query")
	q.Set("list", "allpages")
	q.Set("apnamespace", "0")
	q.Set("apfilterredir", "nonredirects")
	q.Set("format", "json")
	q.Set("aplimit", strconv.Itoa(requestLimit))
	if apContinue != "" {
		q.Set("apcontinue", apContinue)
	} else if strings.TrimSpace(fromTitle) != "" {
		q.Set("apfrom", fromTitle)
	}

	body, err := c.doGet(ctx, q)
	if err != nil {
		return nil, "", err
	}

	var payload struct {
		Continue struct {
			APContinue string `json:"apcontinue"`
		} `json:"continue"`
		Query struct {
			AllPages []struct {
				PageID int64  `json:"pageid"`
				Title  string `json:"title"`
			} `json:"allpages"`
		} `json:"query"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", fmt.Errorf("mediawiki client: decode list pages response: %w", err)
	}

	pages := make([]PageSummary, 0, len(payload.Query.AllPages))
	for _, p := range payload.Query.AllPages {
		if apContinue == "" && fromTitle != "" && p.Title == fromTitle {
			continue
		}
		pages = append(pages, PageSummary{ID: p.PageID, Title: p.Title})
	}
	return pages, payload.Continue.APContinue, nil
}

func (c *HTTPMediaWikiClient) GetPage(ctx context.Context, id int64) (*Page, error) {
	if strings.TrimSpace(c.APIBaseURL) == "" {
		return nil, errors.New("mediawiki client: APIBaseURL is required")
	}
	if id <= 0 {
		return nil, fmt.Errorf("mediawiki client: invalid page id %d", id)
	}

	q := url.Values{}
	q.Set("action", "parse")
	q.Set("pageid", strconv.FormatInt(id, 10))
	q.Set("format", "json")
	q.Set("prop", "wikitext|revid")

	body, err := c.doGet(ctx, q)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Parse struct {
			PageID   int64  `json:"pageid"`
			Title    string `json:"title"`
			Revid    int64  `json:"revid"`
			Wikitext any    `json:"wikitext"`
		} `json:"parse"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("mediawiki client: decode get page response: %w", err)
	}

	if payload.Parse.PageID == 0 {
		return nil, fmt.Errorf("mediawiki client: page %d missing parse.pageid", id)
	}

	text := extractWikitext(payload.Parse.Wikitext)
	page := &Page{
		ID:        payload.Parse.PageID,
		Title:     payload.Parse.Title,
		Revision:  payload.Parse.Revid,
		Wikitext:  text,
		URL:       c.pageURL(payload.Parse.Title),
		UpdatedAt: time.Now().UTC(),
	}
	return page, nil
}

func (c *HTTPMediaWikiClient) doGet(ctx context.Context, query url.Values) ([]byte, error) {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}

	baseURL, err := url.Parse(c.APIBaseURL)
	if err != nil {
		return nil, fmt.Errorf("mediawiki client: invalid APIBaseURL: %w", err)
	}
	q := baseURL.Query()
	for k, vals := range query {
		for _, v := range vals {
			q.Set(k, v)
		}
	}
	baseURL.RawQuery = q.Encode()

	if err := c.respectRateLimit(ctx); err != nil {
		return nil, err
	}

	var lastErr error
	attempts := c.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("mediawiki client: create request: %w", err)
		}
		if ua := strings.TrimSpace(c.UserAgent); ua != "" {
			req.Header.Set("User-Agent", ua)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("mediawiki client: request failed: %w", err)
			if attempt < attempts-1 {
				if err := sleepWithContext(ctx, c.retryDelayFor(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			break
		}

		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("mediawiki client: read response: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("mediawiki client: close response body: %w", closeErr)
		}

		if resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("mediawiki client: server status %d", resp.StatusCode)
			if attempt < attempts-1 {
				if err := sleepWithContext(ctx, c.retryDelayFor(attempt)); err != nil {
					return nil, err
				}
				continue
			}
			break
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("mediawiki client: unexpected status %d", resp.StatusCode)
		}

		return body, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("mediawiki client: request failed")
}

func (c *HTTPMediaWikiClient) respectRateLimit(ctx context.Context) error {
	if c.MinInterval <= 0 {
		return nil
	}
	wait := time.Until(c.lastRequest.Add(c.MinInterval))
	if wait > 0 {
		if err := sleepWithContext(ctx, wait); err != nil {
			return err
		}
	}
	c.lastRequest = time.Now()
	return nil
}

func (c *HTTPMediaWikiClient) retryDelayFor(attempt int) time.Duration {
	delay := c.RetryDelay
	if delay <= 0 {
		delay = 100 * time.Millisecond
	}
	if attempt <= 0 {
		return delay
	}
	return delay * time.Duration(1<<attempt)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func extractWikitext(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case map[string]any:
		if text, ok := v["*"]; ok {
			if s, ok := text.(string); ok {
				return s
			}
		}
		if text, ok := v["text"]; ok {
			if s, ok := text.(string); ok {
				return s
			}
		}
	}
	return ""
}

func (c *HTTPMediaWikiClient) pageURL(title string) string {
	base := strings.TrimSpace(c.PageBaseURL)
	if base == "" {
		return ""
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	titlePath := strings.ReplaceAll(strings.TrimSpace(title), " ", "_")
	if titlePath == "" {
		return base
	}
	return base + url.PathEscape(titlePath)
}
