package ingest

import (
	"strings"
	"testing"
)

func TestCleanWikitextStripsTemplates(t *testing.T) {
	in := "{{Quest Infobox|type=Side|region=Huanglong}}\n\nThe quest begins."
	got := CleanWikitext(in)
	if strings.Contains(got, "Quest Infobox") || strings.Contains(got, "{{") {
		t.Fatalf("template not stripped: %q", got)
	}
	if !strings.Contains(got, "The quest begins.") {
		t.Fatalf("body lost: %q", got)
	}
}

func TestCleanWikitextStripsNestedTemplates(t *testing.T) {
	in := "Reward: {{Item|Astrite|qty={{Number|50}}}} for completion."
	got := CleanWikitext(in)
	if strings.Contains(got, "{{") || strings.Contains(got, "}}") {
		t.Fatalf("nested template not fully stripped: %q", got)
	}
	if !strings.Contains(got, "Reward:") || !strings.Contains(got, "for completion.") {
		t.Fatalf("body damaged: %q", got)
	}
}

func TestCleanWikitextWikilinks(t *testing.T) {
	in := "Visit [[Jinzhou]] in [[Huanglong|the eastern region]]."
	got := CleanWikitext(in)
	if strings.Contains(got, "[[") || strings.Contains(got, "]]") {
		t.Fatalf("wikilink markers remain: %q", got)
	}
	if !strings.Contains(got, "Jinzhou") || !strings.Contains(got, "the eastern region") {
		t.Fatalf("wikilink display text not preserved: %q", got)
	}
}

func TestCleanWikitextStripsFileLinks(t *testing.T) {
	in := "Hero shot: [[File:Chixia.png|thumb|200px|Chixia in combat]]\nDescription follows."
	got := CleanWikitext(in)
	if strings.Contains(got, "File:") || strings.Contains(got, "thumb") {
		t.Fatalf("file link not stripped: %q", got)
	}
	if !strings.Contains(got, "Description follows") {
		t.Fatalf("body lost: %q", got)
	}
}

func TestCleanWikitextStripsRefAndComments(t *testing.T) {
	in := "Released in version 1.0<ref>Patch notes</ref>.<!-- TODO check date -->"
	got := CleanWikitext(in)
	if strings.Contains(got, "<ref") || strings.Contains(got, "</ref>") {
		t.Fatalf("ref tag remains: %q", got)
	}
	if strings.Contains(got, "<!--") || strings.Contains(got, "TODO") {
		t.Fatalf("comment remains: %q", got)
	}
	if !strings.Contains(got, "Released in version 1.0") {
		t.Fatalf("body damaged: %q", got)
	}
}

func TestCleanWikitextStripsHeadingMarkers(t *testing.T) {
	in := "== Backstory ==\n\nThe story begins.\n\n=== Early Life ===\nDetails."
	got := CleanWikitext(in)
	if strings.Contains(got, "==") {
		t.Fatalf("heading markers remain: %q", got)
	}
	if !strings.Contains(got, "Backstory") || !strings.Contains(got, "Early Life") {
		t.Fatalf("heading text lost: %q", got)
	}
}

func TestCleanWikitextStripsBoldItalic(t *testing.T) {
	in := "She is '''very''' strong and ''cunning''."
	got := CleanWikitext(in)
	if strings.Contains(got, "'''") || strings.Contains(got, "''") {
		t.Fatalf("markers remain: %q", got)
	}
	if !strings.Contains(got, "very") || !strings.Contains(got, "cunning") {
		t.Fatalf("text lost: %q", got)
	}
}

func TestCleanWikitextEmpty(t *testing.T) {
	if CleanWikitext("") != "" {
		t.Fatalf("expected empty for empty input")
	}
}
