package websearch

import (
	"testing"
)

func TestIsCanonAuthoritativeFandomTrue(t *testing.T) {
	url := "https://wutheringwaves.fandom.com/wiki/Characters"
	if !IsCanonAuthoritative(url) {
		t.Errorf("IsCanonAuthoritative(%q) = false, want true", url)
	}
}

func TestIsCanonAuthoritativeKuroGamesTrue(t *testing.T) {
	url := "https://kurogames.com/news"
	if !IsCanonAuthoritative(url) {
		t.Errorf("IsCanonAuthoritative(%q) = false, want true", url)
	}
}

func TestIsCanonAuthoritativeRandomFalse(t *testing.T) {
	url := "https://example.com/article"
	if IsCanonAuthoritative(url) {
		t.Errorf("IsCanonAuthoritative(%q) = true, want false", url)
	}
}

func TestIsCanonAuthoritativeMalformedURLFalse(t *testing.T) {
	url := "not a valid url at all"
	if IsCanonAuthoritative(url) {
		t.Errorf("IsCanonAuthoritative(%q) = true, want false", url)
	}
}

func TestIsCanonAuthoritativeEmptyFalse(t *testing.T) {
	if IsCanonAuthoritative("") {
		t.Errorf("IsCanonAuthoritative(\"\") = true, want false")
	}
}
