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

var (
	ErrTimeout         = errors.New("web search timed out")
	ErrProviderFailure = errors.New("web search provider error")
	ErrInvalidResponse = errors.New("web search returned invalid response")
	ErrEmptyQuery      = errors.New("empty query")
)

type HTTPProvider struct {
	ProviderName string
	BaseURL      string
	APIKey       string
	HTTP         *http.Client
	Timeout      time.Duration
}

func NewHTTPProvider(name, baseURL, apiKey string, timeout time.Duration) *HTTPProvider {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &HTTPProvider{
		ProviderName: name,
		BaseURL:      baseURL,
		APIKey:       apiKey,
		HTTP:         &http.Client{Timeout: timeout},
		Timeout:      timeout,
	}
}

type httpResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet"`
	} `json:"results"`
}

func (h *HTTPProvider) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, ErrEmptyQuery
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("limit", fmt.Sprintf("%d", limit))

	reqURL := h.BaseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	if h.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.APIKey)
	}

	resp, err := h.HTTP.Do(req)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	var httpResp httpResponse
	if err := json.Unmarshal(body, &httpResp); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}

	results := make([]SearchResult, 0, len(httpResp.Results))
	for _, r := range httpResp.Results {
		results = append(results, SearchResult{
			Title:         r.Title,
			URL:           r.URL,
			Snippet:       r.Snippet,
			Source:        h.ProviderName,
			Authoritative: IsCanonAuthoritative(r.URL),
		})
	}

	return results, nil
}

func (h *HTTPProvider) Name() string {
	return h.ProviderName
}
