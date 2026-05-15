package wire

import "testing"

func TestSanitizeQueryForEmbeddingStripsTriggerKeyword(t *testing.T) {
	cases := map[string]string{
		"iris, apa itu ATK?":              "apa itu ATK",
		"iris what is ATK":                "what is ATK",
		"iris! siapa Chixia":              "siapa Chixia",
		"hey iris, darimana trophy?":      "hey darimana trophy",
		"<@123456789012345678> apa itu A": "apa itu A",
		"  iris : siapa rover  ":          "siapa rover",
		"iris":                            "",
		"":                                "",
	}
	for in, want := range cases {
		if got := sanitizeQueryForEmbedding(in); got != want {
			t.Errorf("sanitizeQueryForEmbedding(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeQueryForEmbeddingPreservesContent(t *testing.T) {
	in := "What is the ATK formula?"
	got := sanitizeQueryForEmbedding(in)
	if got != "What is the ATK formula" {
		t.Errorf("expected trailing question mark stripped, got %q", got)
	}
}
