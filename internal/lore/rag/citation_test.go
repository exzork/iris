package rag

import "testing"

func TestFormatCitationSingle(t *testing.T) {
	c := Citation{
		Title: "Rover",
		URL:   "https://wutheringwaves.fandom.com/wiki/Rover",
	}

	result := FormatCitation(c)
	expected := `"Rover" - https://wutheringwaves.fandom.com/wiki/Rover`

	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestFormatMultipleBullets(t *testing.T) {
	cs := []Citation{
		{Title: "Rover", URL: "https://wutheringwaves.fandom.com/wiki/Rover"},
		{Title: "Jinhsi", URL: "https://wutheringwaves.fandom.com/wiki/Jinhsi"},
	}

	result := FormatMultiple(cs)
	expected := `- "Rover" - https://wutheringwaves.fandom.com/wiki/Rover
- "Jinhsi" - https://wutheringwaves.fandom.com/wiki/Jinhsi`

	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestFormatMultipleDedupesURLs(t *testing.T) {
	cs := []Citation{
		{Title: "Rover", URL: "https://wutheringwaves.fandom.com/wiki/Rover"},
		{Title: "Rover (alt)", URL: "https://wutheringwaves.fandom.com/wiki/Rover"},
		{Title: "Jinhsi", URL: "https://wutheringwaves.fandom.com/wiki/Jinhsi"},
	}

	result := FormatMultiple(cs)
	expected := `- "Rover" - https://wutheringwaves.fandom.com/wiki/Rover
- "Rover (alt)" - https://wutheringwaves.fandom.com/wiki/Rover
- "Jinhsi" - https://wutheringwaves.fandom.com/wiki/Jinhsi`

	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}
