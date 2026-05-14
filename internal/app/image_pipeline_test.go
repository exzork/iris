package app

import (
	"context"
	"errors"
	"testing"
)

func TestDetectIntentKeywords(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"buatkan gambar Rover", true},
		{"buat gambar karakter", true},
		{"generate image of Rover", true},
		{"lukiskan Rover", true},
		{"render Rover", true},
		{"buat image", true},
		{"apa itu Rover", false},
		{"siapa Rover", false},
		{"gambar apa", false},
	}

	for _, tt := range tests {
		result := DetectIntent(tt.query)
		if result != tt.expected {
			t.Errorf("DetectIntent(%q) = %v, want %v", tt.query, result, tt.expected)
		}
	}
}

func TestGenerateSuccess(t *testing.T) {
	fakeGen := &fakeImageGen{url: "https://example.com/image.png"}
	pipeline := NewImagePipeline(fakeGen)

	result := pipeline.Generate(context.Background(), "buatkan gambar Rover")

	if result.Err != nil {
		t.Errorf("expected no error, got %v", result.Err)
	}
	if result.URL != "https://example.com/image.png" {
		t.Errorf("expected URL https://example.com/image.png, got %s", result.URL)
	}
}

func TestGenerateFailureReturnsErr(t *testing.T) {
	fakeGen := &fakeImageGen{err: errors.New("quota exceeded")}
	pipeline := NewImagePipeline(fakeGen)

	result := pipeline.Generate(context.Background(), "buatkan gambar Rover")

	if result.Err == nil {
		t.Errorf("expected error, got nil")
	}
	if result.URL != "" {
		t.Errorf("expected empty URL on error, got %s", result.URL)
	}
}

func TestGenerateNilPipelineReturnsErr(t *testing.T) {
	var pipeline *ImagePipeline
	result := pipeline.Generate(context.Background(), "test")

	if result.Err != ErrImageUnavailable {
		t.Errorf("expected ErrImageUnavailable, got %v", result.Err)
	}
}

type fakeImageGen struct {
	url string
	err error
}

func (f *fakeImageGen) Generate(ctx context.Context, prompt string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.url, nil
}
