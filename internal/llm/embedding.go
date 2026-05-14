package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type EmbeddingConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	Dimension int
	Timeout   time.Duration
	MaxRetries int
	RetryDelay time.Duration
}

type EmbeddingClient struct {
	config *EmbeddingConfig
	http   *http.Client
}

type embeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

type embeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func NewEmbeddingClient(cfg *EmbeddingConfig) *EmbeddingClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	retryDelay := cfg.RetryDelay
	if retryDelay == 0 {
		retryDelay = 1 * time.Second
	}

	cfg.MaxRetries = maxRetries
	cfg.RetryDelay = retryDelay

	return &EmbeddingClient{
		config: cfg,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *EmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	req := embeddingRequest{
		Input: text,
		Model: c.config.Model,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	var lastErr error
	for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
		resp, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = err
			if attempt < c.config.MaxRetries-1 {
				time.Sleep(c.config.RetryDelay)
			}
			continue
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		var embResp embeddingResponse
		if err := json.Unmarshal(respBody, &embResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if embResp.Error != nil {
			return nil, fmt.Errorf("API error: %s", embResp.Error.Message)
		}

		if len(embResp.Data) == 0 {
			return nil, fmt.Errorf("no embeddings returned in response")
		}

		embedding := embResp.Data[0].Embedding
		if len(embedding) != c.config.Dimension {
			return nil, fmt.Errorf("dimension mismatch: expected %d, got %d", c.config.Dimension, len(embedding))
		}

		return embedding, nil
	}

	return nil, fmt.Errorf("failed after %d retries: %w", c.config.MaxRetries, lastErr)
}
