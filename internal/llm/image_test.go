package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImageClient_GenerateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if req["model"] != "dall-e-3" {
			t.Errorf("expected model dall-e-3, got %v", req["model"])
		}

		response := map[string]interface{}{
			"created": 1234567890,
			"data": []map[string]interface{}{
				{
					"url": "https://example.com/image.png",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewImageClient(&ImageConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "dall-e-3",
	})

	ctx := context.Background()
	url, err := client.Generate(ctx, "a beautiful landscape")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if url != "https://example.com/image.png" {
		t.Errorf("expected URL 'https://example.com/image.png', got %s", url)
	}
}

func TestImageClient_GenerateFailureSilent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Rate limit exceeded",
				"type":    "server_error",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewImageClient(&ImageConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "dall-e-3",
	})

	ctx := context.Background()
	url, err := client.Generate(ctx, "a beautiful landscape")

	if err != nil {
		t.Fatalf("expected silent failure (no error), got: %v", err)
	}

	if url != "" {
		t.Errorf("expected empty URL on failure, got %s", url)
	}
}

func TestImageClient_GenerateEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"created": 1234567890,
			"data":    []map[string]interface{}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewImageClient(&ImageConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "dall-e-3",
	})

	ctx := context.Background()
	url, err := client.Generate(ctx, "a beautiful landscape")

	if err != nil {
		t.Fatalf("expected silent failure (no error), got: %v", err)
	}

	if url != "" {
		t.Errorf("expected empty URL on failure, got %s", url)
	}
}

func TestImageClient_GenerateMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"invalid": "response"}`))
	}))
	defer server.Close()

	client := NewImageClient(&ImageConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "dall-e-3",
	})

	ctx := context.Background()
	url, err := client.Generate(ctx, "a beautiful landscape")

	if err != nil {
		t.Fatalf("expected silent failure (no error), got: %v", err)
	}

	if url != "" {
		t.Errorf("expected empty URL on failure, got %s", url)
	}
}

func TestImageClient_GenerateNetworkError(t *testing.T) {
	client := NewImageClient(&ImageConfig{
		APIKey:  "test-key",
		BaseURL: "http://invalid-host-that-does-not-exist:9999",
		Model:   "dall-e-3",
	})

	ctx := context.Background()
	url, err := client.Generate(ctx, "a beautiful landscape")

	if err != nil {
		t.Fatalf("expected silent failure (no error), got: %v", err)
	}

	if url != "" {
		t.Errorf("expected empty URL on failure, got %s", url)
	}
}
