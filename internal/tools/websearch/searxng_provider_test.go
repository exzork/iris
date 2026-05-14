package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearXNG_ReturnsParsedResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]string{
				{
					"title":   "Wuthering Waves Wiki",
					"url":     "https://wutheringwaves.fandom.com/wiki/Home",
					"content": "Official wiki for Wuthering Waves",
					"engine":  "google",
				},
				{
					"title":   "Example Article",
					"url":     "https://example.com/article",
					"content": "Some random article",
					"engine":  "duckduckgo",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewSearXNGProvider(server.URL, 5*time.Second)
	results, err := provider.Search(context.Background(), "test query", 10)

	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if len(results) != 2 {
		t.Fatalf("Search() returned %d results, want 2", len(results))
	}

	if results[0].Title != "Wuthering Waves Wiki" {
		t.Errorf("results[0].Title = %q, want %q", results[0].Title, "Wuthering Waves Wiki")
	}
	if results[0].URL != "https://wutheringwaves.fandom.com/wiki/Home" {
		t.Errorf("results[0].URL = %q, want %q", results[0].URL, "https://wutheringwaves.fandom.com/wiki/Home")
	}
	if results[0].Snippet != "Official wiki for Wuthering Waves" {
		t.Errorf("results[0].Snippet = %q, want %q", results[0].Snippet, "Official wiki for Wuthering Waves")
	}
	if results[0].Source != "searxng" {
		t.Errorf("results[0].Source = %q, want %q", results[0].Source, "searxng")
	}
	if !results[0].Authoritative {
		t.Errorf("results[0].Authoritative = false, want true")
	}

	if results[1].Authoritative {
		t.Errorf("results[1].Authoritative = true, want false")
	}
}

func TestSearXNG_EmptyQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for empty query")
	}))
	defer server.Close()

	provider := NewSearXNGProvider(server.URL, 5*time.Second)
	_, err := provider.Search(context.Background(), "", 10)

	if err != ErrEmptyQuery {
		t.Errorf("Search() error = %v, want %v", err, ErrEmptyQuery)
	}
}

func TestSearXNG_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewSearXNGProvider(server.URL, 50*time.Millisecond)
	_, err := provider.Search(context.Background(), "test query", 10)

	if err != ErrTimeout {
		t.Errorf("Search() error = %v, want %v", err, ErrTimeout)
	}
}

func TestSearXNG_5xx_ReturnsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewSearXNGProvider(server.URL, 5*time.Second)
	_, err := provider.Search(context.Background(), "test query", 10)

	if err != ErrProviderFailure {
		t.Errorf("Search() error = %v, want %v", err, ErrProviderFailure)
	}
}

func TestSearXNG_BadJSON_ReturnsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	provider := NewSearXNGProvider(server.URL, 5*time.Second)
	_, err := provider.Search(context.Background(), "test query", 10)

	if !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("Search() error = %v, want wrapped %v", err, ErrInvalidResponse)
	}
}

func TestSearXNG_RespectsLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]string{
				{"title": "Result 1", "url": "https://example.com/1", "content": "Content 1", "engine": "google"},
				{"title": "Result 2", "url": "https://example.com/2", "content": "Content 2", "engine": "google"},
				{"title": "Result 3", "url": "https://example.com/3", "content": "Content 3", "engine": "google"},
				{"title": "Result 4", "url": "https://example.com/4", "content": "Content 4", "engine": "google"},
				{"title": "Result 5", "url": "https://example.com/5", "content": "Content 5", "engine": "google"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewSearXNGProvider(server.URL, 5*time.Second)
	results, err := provider.Search(context.Background(), "test query", 2)

	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if len(results) != 2 {
		t.Errorf("Search() returned %d results, want 2", len(results))
	}
}

func TestSearXNG_Name(t *testing.T) {
	provider := NewSearXNGProvider("http://localhost:8080", 5*time.Second)
	if provider.Name() != "searxng" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "searxng")
	}
}
