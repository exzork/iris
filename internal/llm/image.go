package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type ImageConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	MaxRetries int
	RetryDelay time.Duration
}

type ImageClient struct {
	config *ImageConfig
	http   *http.Client
}

type imageRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
	N      int    `json:"n"`
}

type imageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func NewImageClient(cfg *ImageConfig) *ImageClient {
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

	return &ImageClient{
		config: cfg,
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *ImageClient) Generate(ctx context.Context, prompt string) (string, error) {
	req := imageRequest{
		Prompt: prompt,
		Model:  c.config.Model,
		N:      1,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return "", nil
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
		resp, err := c.http.Do(httpReq)
		if err != nil {
			if attempt < c.config.MaxRetries-1 {
				time.Sleep(c.config.RetryDelay)
				continue
			}
			return "", nil
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", nil
		}

		var imgResp imageResponse
		if err := json.Unmarshal(respBody, &imgResp); err != nil {
			return "", nil
		}

		if imgResp.Error != nil {
			return "", nil
		}

		if len(imgResp.Data) == 0 {
			return "", nil
		}

		return imgResp.Data[0].URL, nil
	}

	return "", nil
}
