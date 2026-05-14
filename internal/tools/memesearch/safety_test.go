package memesearch

import (
	"testing"
)

func TestClassifyNSFWKeywordInURL(t *testing.T) {
	classifier := NewDefaultSafetyClassifier()
	item := MediaItem{
		URL:      "https://example.com/nsfw-content.gif",
		Source:   SourceX,
		MimeType: "image/gif",
		Caption:  "meme",
		Safety:   SafetyUnknown,
	}

	result := classifier.Classify(item)
	if result != SafetyNSFW {
		t.Errorf("expected SafetyNSFW, got %s", result)
	}
}

func TestClassifyNSFWKeywordInCaption(t *testing.T) {
	classifier := NewDefaultSafetyClassifier()
	item := MediaItem{
		URL:      "https://example.com/meme.gif",
		Source:   SourceX,
		MimeType: "image/gif",
		Caption:  "this is porn content",
		Safety:   SafetyUnknown,
	}

	result := classifier.Classify(item)
	if result != SafetyNSFW {
		t.Errorf("expected SafetyNSFW, got %s", result)
	}
}

func TestClassifyTenorSafe(t *testing.T) {
	classifier := NewDefaultSafetyClassifier()
	item := MediaItem{
		URL:      "https://media.tenor.com/example.gif",
		Source:   SourceDiscordHistory,
		MimeType: "image/gif",
		Caption:  "funny reaction",
		Safety:   SafetyUnknown,
	}

	result := classifier.Classify(item)
	if result != SafetySafe {
		t.Errorf("expected SafetySafe, got %s", result)
	}
}

func TestClassifyUnknownHost(t *testing.T) {
	classifier := NewDefaultSafetyClassifier()
	item := MediaItem{
		URL:      "https://unknown-host.com/meme.gif",
		Source:   SourceX,
		MimeType: "image/gif",
		Caption:  "funny meme",
		Safety:   SafetyUnknown,
	}

	result := classifier.Classify(item)
	if result != SafetyUnknown {
		t.Errorf("expected SafetyUnknown, got %s", result)
	}
}
