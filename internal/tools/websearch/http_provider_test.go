package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProviderNormalizesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]string{
				{
					"title":   "Wuthering Waves Wiki",
					"url":     "https://wutheringwaves.fandom.com/wiki/Home",
					"snippet": "Official wiki for Wuthering Waves",
				},
				{
					"title":   "Example Article",
					"url":     "https://example.com/article",
					"snippet": "Some random article",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewHTTPProvider("test", server.URL, "", 5*time.Second)
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
	if results[0].Source != "test" {
		t.Errorf("results[0].Source = %q, want %q", results[0].Source, "test")
	}
	if !results[0].Authoritative {
		t.Errorf("results[0].Authoritative = false, want true")
	}

	if results[1].Authoritative {
		t.Errorf("results[1].Authoritative = true, want false")
	}
}

func TestHTTPProviderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewHTTPProvider("test", server.URL, "", 50*time.Millisecond)
	_, err := provider.Search(context.Background(), "test query", 10)

	if err != ErrTimeout {
		t.Errorf("Search() error = %v, want %v", err, ErrTimeout)
	}
}

func TestHTTPProvider5xxReturnsProviderFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := NewHTTPProvider("test", server.URL, "", 5*time.Second)
	_, err := provider.Search(context.Background(), "test query", 10)

	if err != ErrProviderFailure {
		t.Errorf("Search() error = %v, want %v", err, ErrProviderFailure)
	}
}

func TestHTTPProviderInvalidJSONReturnsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	provider := NewHTTPProvider("test", server.URL, "", 5*time.Second)
	_, err := provider.Search(context.Background(), "test query", 10)

	if err == nil {
		t.Errorf("Search() error = nil, want ErrInvalidResponse")
	}
}

func TestHTTPProviderSetsAuthoritativeFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"results": []map[string]string{
				{
					"title":   "Fandom Result",
					"url":     "https://wutheringwaves.fandom.com/wiki/Test",
					"snippet": "From fandom",
				},
				{
					"title":   "Random Result",
					"url":     "https://example.com/test",
					"snippet": "From example",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewHTTPProvider("test", server.URL, "", 5*time.Second)
	results, err := provider.Search(context.Background(), "test", 10)

	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if !results[0].Authoritative {
		t.Errorf("results[0].Authoritative = false, want true for fandom URL")
	}

	if results[1].Authoritative {
		t.Errorf("results[1].Authoritative = true, want false for example.com URL")
	}
}
