package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChatStream_Emits_DeltasInOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream, got %s", r.Header.Get("Accept"))
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if req["stream"] != true {
			t.Errorf("expected stream true, got %v", req["stream"])
		}

		w.Header().Set("Content-Type", "text/event-stream")

		// Emit 4 deltas: "Ha", "lo", " d", "unia"
		deltas := []string{"Ha", "lo", " d", "unia"}
		for _, delta := range deltas {
			chunk := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"delta": map[string]interface{}{
							"content": delta,
						},
						"finish_reason": nil,
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
		}

		// Emit [DONE]
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "test-model",
		Temperature: 0.7,
		MaxTokens:   100,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	var deltas []string
	var onDoneCalled bool
	var onErrorCalled bool

	cb := StreamCallbacks{
		OnDelta: func(text string) {
			deltas = append(deltas, text)
		},
		OnDone: func() {
			onDoneCalled = true
		},
		OnError: func(err error) {
			onErrorCalled = true
			t.Errorf("unexpected error: %v", err)
		},
	}

	finalText, err := client.ChatStream(ctx, "test-model", 123, messages, cb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(deltas) != 4 {
		t.Errorf("expected 4 deltas, got %d", len(deltas))
	}
	if deltas[0] != "Ha" || deltas[1] != "lo" || deltas[2] != " d" || deltas[3] != "unia" {
		t.Errorf("deltas out of order or incorrect: %v", deltas)
	}
	if finalText != "Halo dunia" {
		t.Errorf("expected 'Halo dunia', got %s", finalText)
	}
	if !onDoneCalled {
		t.Errorf("OnDone was not called")
	}
	if onErrorCalled {
		t.Errorf("OnError should not have been called")
	}
}

func TestChatStream_HandlesDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		chunk := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]interface{}{
						"content": "test",
					},
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(data))

		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	onDoneCount := 0
	cb := StreamCallbacks{
		OnDelta: func(text string) {},
		OnDone: func() {
			onDoneCount++
		},
		OnError: func(err error) {
			t.Errorf("unexpected error: %v", err)
		},
	}

	_, err := client.ChatStream(ctx, "test-model", 123, messages, cb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if onDoneCount != 1 {
		t.Errorf("expected OnDone to be called exactly once, got %d", onDoneCount)
	}
}

func TestChatStream_NetworkError_CallsOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// Close connection without sending [DONE]
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("hijacker not supported")
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "test-model",
		MaxRetries:  0,
		RetryDelay:  10 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	var onErrorCalled bool
	var errorReceived error

	cb := StreamCallbacks{
		OnDelta: func(text string) {},
		OnDone: func() {
			t.Errorf("OnDone should not be called on network error")
		},
		OnError: func(err error) {
			onErrorCalled = true
			errorReceived = err
		},
	}

	finalText, err := client.ChatStream(ctx, "test-model", 123, messages, cb)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if finalText != "" {
		t.Errorf("expected empty finalText on error, got %s", finalText)
	}
	if !onErrorCalled {
		t.Errorf("OnError was not called")
	}
	if errorReceived == nil {
		t.Errorf("expected error in OnError callback, got nil")
	}
}

func TestChatStream_CtxCancellation_StopsStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		// Send first delta
		chunk := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]interface{}{
						"content": "first",
					},
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		w.(http.Flusher).Flush()

		// Wait a bit to allow context cancellation
		time.Sleep(100 * time.Millisecond)

		// Try to send more (but context should be cancelled)
		chunk2 := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]interface{}{
						"content": "second",
					},
					"finish_reason": nil,
				},
			},
		}
		data2, _ := json.Marshal(chunk2)
		fmt.Fprintf(w, "data: %s\n\n", string(data2))
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})

	ctx, cancel := context.WithCancel(context.Background())
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	deltaCount := 0
	cb := StreamCallbacks{
		OnDelta: func(text string) {
			deltaCount++
			if deltaCount == 1 {
				// Cancel after first delta
				cancel()
			}
		},
		OnDone: func() {
			t.Errorf("OnDone should not be called after cancellation")
		},
		OnError: func(err error) {
			// Expected: context.Canceled or similar
		},
	}

	_, err := client.ChatStream(ctx, "test-model", 123, messages, cb)
	if err == nil {
		t.Errorf("expected context cancellation error, got nil")
	}

	if deltaCount > 1 {
		t.Errorf("expected at most 1 delta after cancellation, got %d", deltaCount)
	}
}

func TestChatStream_RetriesOnFirstByteFailure(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount == 1 {
			// First attempt: fail with 500 before any body
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Second attempt: succeed
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]interface{}{
						"content": "success",
					},
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		Model:      "test-model",
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	deltaCount := 0
	cb := StreamCallbacks{
		OnDelta: func(text string) {
			deltaCount++
		},
		OnDone: func() {},
		OnError: func(err error) {
			t.Errorf("unexpected error: %v", err)
		},
	}

	finalText, err := client.ChatStream(ctx, "test-model", 123, messages, cb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if attemptCount != 2 {
		t.Errorf("expected 2 attempts, got %d", attemptCount)
	}
	if deltaCount != 1 {
		t.Errorf("expected 1 delta (from second attempt), got %d", deltaCount)
	}
	if finalText != "success" {
		t.Errorf("expected 'success', got %s", finalText)
	}
}

func TestChatStream_IgnoresEmptyDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		// Send delta with content
		chunk1 := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]interface{}{
						"content": "hello",
					},
					"finish_reason": nil,
				},
			},
		}
		data1, _ := json.Marshal(chunk1)
		fmt.Fprintf(w, "data: %s\n\n", string(data1))

		// Send delta with empty object (no content field)
		chunk2 := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta":         map[string]interface{}{},
					"finish_reason": nil,
				},
			},
		}
		data2, _ := json.Marshal(chunk2)
		fmt.Fprintf(w, "data: %s\n\n", string(data2))

		// Send another delta with content
		chunk3 := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]interface{}{
						"content": " world",
					},
					"finish_reason": nil,
				},
			},
		}
		data3, _ := json.Marshal(chunk3)
		fmt.Fprintf(w, "data: %s\n\n", string(data3))

		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	var deltas []string
	cb := StreamCallbacks{
		OnDelta: func(text string) {
			deltas = append(deltas, text)
		},
		OnDone: func() {},
		OnError: func(err error) {
			t.Errorf("unexpected error: %v", err)
		},
	}

	finalText, err := client.ChatStream(ctx, "test-model", 123, messages, cb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(deltas) != 2 {
		t.Errorf("expected 2 deltas (empty delta ignored), got %d: %v", len(deltas), deltas)
	}
	if deltas[0] != "hello" || deltas[1] != " world" {
		t.Errorf("expected ['hello', ' world'], got %v", deltas)
	}
	if finalText != "hello world" {
		t.Errorf("expected 'hello world', got %s", finalText)
	}
}

func TestChatStream_NoRetryAfterFirstDelta(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/event-stream")

		if requestCount == 1 {
			chunk := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"delta": map[string]interface{}{
							"content": "First delta",
						},
						"finish_reason": nil,
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			w.(http.Flusher).Flush()

			http.Error(w, "connection lost", http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "test-model",
		MaxRetries:  3,
		RetryDelay:  10 * time.Millisecond,
	})

	ctx := context.Background()
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	var deltas []string

	cb := StreamCallbacks{
		OnDelta: func(text string) {
			deltas = append(deltas, text)
		},
		OnDone: func() {},
		OnError: func(err error) {
		},
	}

	_, err := client.ChatStream(ctx, "test-model", 123, messages, cb)
	if err == nil {
		t.Fatalf("expected error after first delta, got nil")
	}

	if requestCount != 1 {
		t.Errorf("expected exactly 1 request (no retry after first delta), got %d", requestCount)
	}

	if len(deltas) != 1 || deltas[0] != "First delta" {
		t.Errorf("expected 1 delta 'First delta', got %v", deltas)
	}
}

func TestChatStream_CtxCancelled_AbortsScannerWithinOneSecond(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		chunk := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"delta": map[string]interface{}{
						"content": "Initial",
					},
					"finish_reason": nil,
				},
			},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(data))
		w.(http.Flusher).Flush()

		time.Sleep(10 * time.Second)
	}))
	defer server.Close()

	client := NewClient(&Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
	})

	ctx, cancel := context.WithCancel(context.Background())
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
	}

	var deltas []string
	cb := StreamCallbacks{
		OnDelta: func(text string) {
			deltas = append(deltas, text)
		},
		OnDone: func() {},
		OnError: func(err error) {},
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := client.ChatStream(ctx, "test-model", 123, messages, cb)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}

	if elapsed > 1500*time.Millisecond {
		t.Errorf("expected cancellation within 1.5s, took %v", elapsed)
	}

	if len(deltas) != 1 || deltas[0] != "Initial" {
		t.Errorf("expected 1 delta before cancellation, got %v", deltas)
	}
}
