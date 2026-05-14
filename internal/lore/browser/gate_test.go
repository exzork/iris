package browser

import (
	"testing"

	loresource "github.com/eko/iris-bot/internal/lore/source"
)

func TestGateAllowFandomPolicy(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}

	err := gate.Allow("https://wutheringwaves.fandom.com/wiki/Rover")
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestGateRejectUnregisteredHost(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}

	err := gate.Allow("https://example.com/foo")
	if err != ErrHostNotRegistered {
		t.Errorf("expected ErrHostNotRegistered, got %v", err)
	}
}

func TestGateRejectMethodNotAllowedByPolicy(t *testing.T) {
	registry := loresource.NewRegistry()
	if err := registry.Register(&loresource.Source{
		ID:   "test_source",
		Host: "test.invalid",
		Policy: loresource.Policy{
			Name:               "Test Source",
			License:            "CC BY-SA 3.0",
			AttributionURL:     "https://test.invalid",
			UserAgent:          "TestBot/1.0",
			RateLimitRPS:       1.0,
			AllowedMethods:     []loresource.AccessMethod{loresource.MethodMediaWikiAPI},
			RequiresAttribution: true,
			NotesURL:           "https://test.invalid/tos",
		},
	}); err != nil {
		t.Fatalf("failed to register source: %v", err)
	}

	gate := &Gate{Registry: registry}
	err := gate.Allow("https://test.invalid/page")
	if err != ErrMethodNotAllowed {
		t.Errorf("expected ErrMethodNotAllowed, got %v", err)
	}
}

func TestGateRejectMalformedURL(t *testing.T) {
	registry := loresource.DefaultRegistry()
	gate := &Gate{Registry: registry}

	err := gate.Allow("ht!tp://[invalid")
	if err == nil {
		t.Error("expected error for malformed URL")
	}
}
