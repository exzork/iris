package memesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type GiphyAdapter struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

func NewGiphyAdapter(apiKey string) *GiphyAdapter {
	return &GiphyAdapter{
		APIKey:  apiKey,
		BaseURL: "https://api.giphy.com/v1",
		HTTP:    &http.Client{Timeout: 8 * time.Second},
	}
}

func (a *GiphyAdapter) Source() Source {
	return SourceGiphy
}

func (a *GiphyAdapter) Search(ctx context.Context, query string, limit int) ([]MediaItem, error) {
	if strings.TrimSpace(a.APIKey) == "" {
		return nil, fmt.Errorf("giphy: api key not configured")
	}
	if limit <= 0 || limit > 25 {
		limit = 5
	}

	endpoint, err := url.Parse(a.BaseURL + "/gifs/search")
	if err != nil {
		return nil, fmt.Errorf("giphy: bad base url: %w", err)
	}
	q := endpoint.Query()
	q.Set("api_key", a.APIKey)
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("rating", "pg-13")
	q.Set("lang", "en")
	q.Set("bundle", "messaging_non_clips")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("giphy: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := a.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("giphy: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("giphy: status %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Data []struct {
			Title  string `json:"title"`
			Images struct {
				Original struct {
					URL string `json:"url"`
				} `json:"original"`
				Downsized struct {
					URL string `json:"url"`
				} `json:"downsized"`
				FixedHeight struct {
					URL string `json:"url"`
				} `json:"fixed_height"`
			} `json:"images"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("giphy: decode: %w", err)
	}

	results := make([]MediaItem, 0, len(payload.Data))
	for _, gif := range payload.Data {
		mediaURL := gif.Images.Downsized.URL
		if mediaURL == "" {
			mediaURL = gif.Images.FixedHeight.URL
		}
		if mediaURL == "" {
			mediaURL = gif.Images.Original.URL
		}
		if mediaURL == "" {
			continue
		}
		results = append(results, MediaItem{
			URL:      mediaURL,
			Source:   SourceGiphy,
			MimeType: "image/gif",
			Caption:  gif.Title,
			Safety:   SafetySafe,
		})
	}
	return results, nil
}
