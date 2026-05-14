package llm

import (
	"context"
	"errors"
	"testing"
)

type fakeChatClient struct {
	response string
	err      error
}

func (f *fakeChatClient) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	return f.response, f.err
}

func TestClassifyReturnsStrongWhenModelSaysStrong(t *testing.T) {
	fake := &fakeChatClient{response: "STRONG\n"}
	router := &TierRouter{
		Classifier: fake,
		Router:     "kr/claude-haiku-4.5",
		Default:    "kr/claude-sonnet-4.5",
		Strong:     "kr/claude-opus-4.7",
	}

	tier, err := router.Classify(context.Background(), 123, "Jelaskan lore Wuthering Waves secara mendalam")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TierStrong {
		t.Errorf("expected TierStrong, got %v", tier)
	}
}

func TestClassifyReturnsDefaultOnEmpty(t *testing.T) {
	fake := &fakeChatClient{response: "DEFAULT"}
	router := &TierRouter{
		Classifier: fake,
		Router:     "kr/claude-haiku-4.5",
		Default:    "kr/claude-sonnet-4.5",
		Strong:     "kr/claude-opus-4.7",
	}

	tier, err := router.Classify(context.Background(), 123, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TierDefault {
		t.Errorf("expected TierDefault, got %v", tier)
	}
}

func TestClassifyReturnsDefaultOnClassifierError(t *testing.T) {
	fake := &fakeChatClient{err: errors.New("classifier unavailable")}
	router := &TierRouter{
		Classifier: fake,
		Router:     "kr/claude-haiku-4.5",
		Default:    "kr/claude-sonnet-4.5",
		Strong:     "kr/claude-opus-4.7",
	}

	tier, err := router.Classify(context.Background(), 123, "test query")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if tier != TierDefault {
		t.Errorf("expected TierDefault on error, got %v", tier)
	}
	if !errors.Is(err, errors.New("classifier failed")) && err.Error() != "classifier failed: classifier unavailable" {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestClassifyReturnsDefaultWhenUnknown(t *testing.T) {
	fake := &fakeChatClient{response: "maybe"}
	router := &TierRouter{
		Classifier: fake,
		Router:     "kr/claude-haiku-4.5",
		Default:    "kr/claude-sonnet-4.5",
		Strong:     "kr/claude-opus-4.7",
	}

	tier, err := router.Classify(context.Background(), 123, "test query")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != TierDefault {
		t.Errorf("expected TierDefault for unknown response, got %v", tier)
	}
}

func TestModelForMapping(t *testing.T) {
	router := &TierRouter{
		Classifier: nil,
		Router:     "kr/claude-haiku-4.5",
		Default:    "kr/claude-sonnet-4.5",
		Strong:     "kr/claude-opus-4.7",
	}

	tests := []struct {
		tier     Tier
		expected string
	}{
		{TierDefault, "kr/claude-sonnet-4.5"},
		{TierStrong, "kr/claude-opus-4.7"},
	}

	for _, tt := range tests {
		model := router.ModelFor(tt.tier)
		if model != tt.expected {
			t.Errorf("ModelFor(%v) = %s, expected %s", tt.tier, model, tt.expected)
		}
	}
}
