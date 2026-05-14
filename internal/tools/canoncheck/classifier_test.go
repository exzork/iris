package canoncheck

import (
	"testing"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
)

func TestClassifyNoChunksUnsupported(t *testing.T) {
	claim := Claim{Text: "Rover appears in Quest Y"}
	chunks := []ragpkg.ScoredChunk{}

	status := ClassifyContent(chunks, claim, 2, 0.6)

	if status != StatusUnsupported {
		t.Errorf("expected StatusUnsupported, got %s", status)
	}
}

func TestClassifyInsufficientChunksNeedsMore(t *testing.T) {
	claim := Claim{Text: "Rover appears in Quest Y"}
	chunks := []ragpkg.ScoredChunk{
		{Chunk: ragpkg.Chunk{Content: "Rover is a character"}, Score: 0.5},
	}

	status := ClassifyContent(chunks, claim, 2, 0.6)

	if status != StatusNeedsMoreSources {
		t.Errorf("expected StatusNeedsMoreSources, got %s", status)
	}
}

func TestClassifyStrongMatchSupported(t *testing.T) {
	claim := Claim{Text: "Rover appears in Quest Y"}
	chunks := []ragpkg.ScoredChunk{
		{Chunk: ragpkg.Chunk{Content: "Rover appears in Quest Y as a main character"}, Score: 0.85},
		{Chunk: ragpkg.Chunk{Content: "Quest Y features Rover prominently"}, Score: 0.75},
	}

	status := ClassifyContent(chunks, claim, 2, 0.6)

	if status != StatusSupported {
		t.Errorf("expected StatusSupported, got %s", status)
	}
}

func TestClassifyNegationPatternContradicted(t *testing.T) {
	claim := Claim{Text: "Rover appears in Quest Y"}
	chunks := []ragpkg.ScoredChunk{
		{Chunk: ragpkg.Chunk{Content: "Rover tidak muncul di Quest Y"}, Score: 0.8},
	}

	status := ClassifyContent(chunks, claim, 1, 0.6)

	if status != StatusContradicted {
		t.Errorf("expected StatusContradicted, got %s", status)
	}
}
