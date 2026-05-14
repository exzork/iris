package patchnotes

import (
	"testing"
)

func TestClassifyKuroGamesOfficial(t *testing.T) {
	tests := []string{
		"https://kurogames.com/patch-notes",
		"https://www.kurogames.com/news",
		"http://kurogames.com",
	}
	for _, url := range tests {
		got := ClassifySource(url)
		if got != LevelOfficial {
			t.Errorf("ClassifySource(%q) = %v, want %v", url, got, LevelOfficial)
		}
	}
}

func TestClassifyWutheringWavesOfficial(t *testing.T) {
	tests := []string{
		"https://wutheringwaves.com/patch-notes",
		"https://www.wutheringwaves.com/news",
		"http://wutheringwaves.com",
	}
	for _, url := range tests {
		got := ClassifySource(url)
		if got != LevelOfficial {
			t.Errorf("ClassifySource(%q) = %v, want %v", url, got, LevelOfficial)
		}
	}
}

func TestClassifyFandomWiki(t *testing.T) {
	tests := []string{
		"https://wutheringwaves.fandom.com/wiki/Patch",
		"https://www.wutheringwaves.fandom.com/wiki/News",
		"http://wutheringwaves.fandom.com",
	}
	for _, url := range tests {
		got := ClassifySource(url)
		if got != LevelWiki {
			t.Errorf("ClassifySource(%q) = %v, want %v", url, got, LevelWiki)
		}
	}
}

func TestClassifyRedditCommunity(t *testing.T) {
	tests := []string{
		"https://reddit.com/r/WutheringWaves",
		"https://www.reddit.com/r/gaming",
		"http://reddit.com",
	}
	for _, url := range tests {
		got := ClassifySource(url)
		if got != LevelCommunity {
			t.Errorf("ClassifySource(%q) = %v, want %v", url, got, LevelCommunity)
		}
	}
}

func TestClassifyTwitterCommunity(t *testing.T) {
	tests := []string{
		"https://x.com/user/status",
		"https://twitter.com/user",
		"https://www.twitter.com/news",
	}
	for _, url := range tests {
		got := ClassifySource(url)
		if got != LevelCommunity {
			t.Errorf("ClassifySource(%q) = %v, want %v", url, got, LevelCommunity)
		}
	}
}

func TestClassifyYoutubeCommunity(t *testing.T) {
	tests := []string{
		"https://youtube.com/watch?v=abc",
		"https://www.youtube.com/channel/xyz",
	}
	for _, url := range tests {
		got := ClassifySource(url)
		if got != LevelCommunity {
			t.Errorf("ClassifySource(%q) = %v, want %v", url, got, LevelCommunity)
		}
	}
}

func TestClassifyMalformedURLCommunity(t *testing.T) {
	tests := []string{
		"not a url",
		"",
		"ht!tp://invalid",
	}
	for _, url := range tests {
		got := ClassifySource(url)
		if got != LevelCommunity {
			t.Errorf("ClassifySource(%q) = %v, want %v", url, got, LevelCommunity)
		}
	}
}

func TestClassifyUnknownHostCommunity(t *testing.T) {
	tests := []string{
		"https://example.com/news",
		"https://somerandomblog.com/patch",
	}
	for _, url := range tests {
		got := ClassifySource(url)
		if got != LevelCommunity {
			t.Errorf("ClassifySource(%q) = %v, want %v", url, got, LevelCommunity)
		}
	}
}
