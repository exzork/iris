package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// SearXNGProvider implements Provider using a SearXNG JSON endpoint.
// SearXNG responds at GET <BaseURL>/search?q=<query>&format=json
// with {"results":[{"title","url","content","engine", ...}], ...}.
type SearXNGProvider struct {
	BaseURL string        // e.g. "http://searxng:8080"
	HTTP    *http.Client
	Timeout time.Duration
}

// searxngResponse represents the JSON response from SearXNG.
type searxngResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
		Engine  string `json:"engine"`
	} `json:"results"`
}

// NewSearXNGProvider creates a new SearXNG provider.
func NewSearXNGProvider(baseURL string, timeout time.Duration) *SearXNGProvider {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &SearXNGProvider{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: timeout},
		Timeout: timeout,
	}
}

// Search performs a search query against SearXNG and returns results.
func (s *SearXNGProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, ErrEmptyQuery
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")

	reqURL := s.BaseURL + "/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	resp, err := s.HTTP.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrProviderFailure, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return nil, ErrProviderFailure
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrProviderFailure, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	var searxngResp searxngResponse
	if err := json.Unmarshal(body, &searxngResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	results := make([]SearchResult, 0, len(searxngResp.Results))
	for i, r := range searxngResp.Results {
		if i >= limit {
			break
		}
		results = append(results, SearchResult{
			Title:         r.Title,
			URL:           r.URL,
			Snippet:       r.Content,
			Source:        "searxng",
			Authoritative: IsCanonAuthoritative(r.URL),
		})
	}

	return results, nil
}

// Name returns the provider's name.
func (s *SearXNGProvider) Name() string {
	return "searxng"
}
