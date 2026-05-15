package memesearch

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type GiphyAdapter struct {
	APIKey        string
	BaseURL       string
	HTTP          *http.Client
	CandidatePool int
}

func NewGiphyAdapter(apiKey string) *GiphyAdapter {
	return &GiphyAdapter{
		APIKey:        apiKey,
		BaseURL:       "https://api.giphy.com/v1",
		HTTP:          &http.Client{Timeout: 8 * time.Second},
		CandidatePool: 25,
	}
}

func (a *GiphyAdapter) Source() Source {
	return SourceGiphy
}

// Search fetches the top CandidatePool GIFs for the query, shuffles them, and
// returns up to `limit` so repeated invocations of the same query do not
// always pick the same GIF. CandidatePool defaults to 25 (Giphy's max for
// the search endpoint with the default rating).
func (a *GiphyAdapter) Search(ctx context.Context, query string, limit int) ([]MediaItem, error) {
	if strings.TrimSpace(a.APIKey) == "" {
		return nil, fmt.Errorf("giphy: api key not configured")
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > 25 {
		limit = 25
	}

	pool := a.CandidatePool
	if pool <= 0 {
		pool = 25
	}
	if pool < limit {
		pool = limit
	}
	if pool > 50 {
		pool = 50
	}

	endpoint, err := url.Parse(a.BaseURL + "/gifs/search")
	if err != nil {
		return nil, fmt.Errorf("giphy: bad base url: %w", err)
	}
	q := endpoint.Query()
	q.Set("api_key", a.APIKey)
	q.Set("q", query)
	q.Set("limit", strconv.Itoa(pool))
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

	candidates := make([]MediaItem, 0, len(payload.Data))
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
		candidates = append(candidates, MediaItem{
			URL:      mediaURL,
			Source:   SourceGiphy,
			MimeType: "image/gif",
			Caption:  gif.Title,
			Safety:   SafetySafe,
		})
	}
	return shuffleAndTake(candidates, limit), nil
}

func shuffleAndTake(items []MediaItem, n int) []MediaItem {
	if len(items) == 0 || n <= 0 {
		return nil
	}
	if n >= len(items) {
		n = len(items)
	}
	rng := newRand()
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	return items[:n]
}

func newRand() *mrand.Rand {
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err == nil {
		return mrand.New(mrand.NewSource(int64(binary.LittleEndian.Uint64(seed[:]))))
	}
	return mrand.New(mrand.NewSource(time.Now().UnixNano()))
}
