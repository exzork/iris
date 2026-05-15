package orchestrator

import (
	"strings"
	"testing"
)

func TestExtractMediaURLsGiphy(t *testing.T) {
	in := "oke ekspresi ketawa nih:\n\nhttps://media2.giphy.com/media/v1.Y2lkPWFiYw/XHeLeuirRbwptHhSWd/200.gif"
	text, urls := extractMediaURLs(in)
	if len(urls) != 1 {
		t.Fatalf("expected 1 url, got %d: %v", len(urls), urls)
	}
	if !strings.Contains(urls[0], "giphy.com") {
		t.Errorf("url not preserved: %q", urls[0])
	}
	if strings.Contains(text, "giphy.com") {
		t.Errorf("url not stripped from text: %q", text)
	}
	if text != "oke ekspresi ketawa nih:" {
		t.Errorf("unexpected text: %q", text)
	}
}

func TestExtractMediaURLsTenor(t *testing.T) {
	in := "rover salute https://tenor.com/view/rover-salute-12345.gif yo"
	text, urls := extractMediaURLs(in)
	if len(urls) != 1 || !strings.Contains(urls[0], "tenor.com") {
		t.Fatalf("expected 1 tenor url, got %v", urls)
	}
	if strings.Contains(text, "tenor.com") {
		t.Errorf("url not stripped: %q", text)
	}
	if text != "rover salute yo" {
		t.Errorf("text not normalized: %q", text)
	}
}

func TestExtractMediaURLsDiscordSticker(t *testing.T) {
	in := "stik nih https://cdn.discordapp.com/stickers/12345.png done"
	text, urls := extractMediaURLs(in)
	if len(urls) != 1 || !strings.Contains(urls[0], "cdn.discordapp.com/stickers/") {
		t.Fatalf("expected 1 sticker url, got %v", urls)
	}
	if strings.Contains(text, "cdn.discordapp.com") {
		t.Errorf("url not stripped: %q", text)
	}
}

func TestExtractMediaURLsBareImageExtension(t *testing.T) {
	in := "lihat https://example.com/foo.gif kan"
	text, urls := extractMediaURLs(in)
	if len(urls) != 1 || urls[0] != "https://example.com/foo.gif" {
		t.Fatalf("expected gif url, got %v", urls)
	}
	if strings.Contains(text, "example.com") {
		t.Errorf("url not stripped: %q", text)
	}
}

func TestExtractMediaURLsNoMatch(t *testing.T) {
	in := "halo, ini balasan biasa tanpa media"
	text, urls := extractMediaURLs(in)
	if len(urls) != 0 {
		t.Errorf("expected 0 urls, got %v", urls)
	}
	if text != in {
		t.Errorf("text changed: %q vs %q", text, in)
	}
}

func TestExtractMediaURLsLeavesWikiURLs(t *testing.T) {
	in := "lihat wiki https://wutheringwaves.fandom.com/wiki/Rover dong"
	text, urls := extractMediaURLs(in)
	if len(urls) != 0 {
		t.Errorf("wiki URLs should not be extracted as media, got %v", urls)
	}
	if text != in {
		t.Errorf("text changed unexpectedly: %q", text)
	}
}

func TestExtractMediaURLsDeduplicates(t *testing.T) {
	in := "https://media.giphy.com/media/x/200.gif again https://media.giphy.com/media/x/200.gif"
	_, urls := extractMediaURLs(in)
	if len(urls) != 1 {
		t.Errorf("expected dedup to 1 url, got %d: %v", len(urls), urls)
	}
}

func TestExtractMediaURLsTrailingPunctuation(t *testing.T) {
	in := "ketawa https://media.giphy.com/media/x/200.gif."
	_, urls := extractMediaURLs(in)
	if len(urls) != 1 || strings.HasSuffix(urls[0], ".") {
		t.Errorf("trailing dot not stripped: %v", urls)
	}
}
