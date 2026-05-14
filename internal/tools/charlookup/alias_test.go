package charlookup

import (
	"testing"
)

func TestAliasCaseInsensitive(t *testing.T) {
	idx := NewAliasIndex()
	char := &Character{
		ID:      1,
		Name:    "Rover",
		Aliases: []string{"protagonist", "MC"},
	}
	idx.Add(char)

	tests := []struct {
		query string
		found bool
	}{
		{"rover", true},
		{"ROVER", true},
		{"Rover", true},
		{"protagonist", true},
		{"PROTAGONIST", true},
		{"mc", true},
		{"MC", true},
	}

	for _, tt := range tests {
		id, ok := idx.Resolve(tt.query)
		if ok != tt.found {
			t.Errorf("Resolve(%q): found=%v, want %v", tt.query, ok, tt.found)
		}
		if ok && id != char.ID {
			t.Errorf("Resolve(%q): id=%d, want %d", tt.query, id, char.ID)
		}
	}
}

func TestAliasResolveExactName(t *testing.T) {
	idx := NewAliasIndex()
	char := &Character{
		ID:   42,
		Name: "Jinshi",
	}
	idx.Add(char)

	id, ok := idx.Resolve("Jinshi")
	if !ok {
		t.Fatal("Resolve(Jinshi): not found")
	}
	if id != 42 {
		t.Errorf("Resolve(Jinshi): id=%d, want 42", id)
	}
}

func TestAliasNotFound(t *testing.T) {
	idx := NewAliasIndex()
	char := &Character{
		ID:   1,
		Name: "Rover",
	}
	idx.Add(char)

	id, ok := idx.Resolve("NonExistent")
	if ok {
		t.Errorf("Resolve(NonExistent): found=%v, want false", ok)
	}
	if id != 0 {
		t.Errorf("Resolve(NonExistent): id=%d, want 0", id)
	}
}

func TestAliasEmptyQuery(t *testing.T) {
	idx := NewAliasIndex()
	char := &Character{
		ID:   1,
		Name: "Rover",
	}
	idx.Add(char)

	id, ok := idx.Resolve("")
	if ok {
		t.Errorf("Resolve(empty): found=%v, want false", ok)
	}
	if id != 0 {
		t.Errorf("Resolve(empty): id=%d, want 0", id)
	}
}

func TestAliasWhitespaceHandling(t *testing.T) {
	idx := NewAliasIndex()
	char := &Character{
		ID:   1,
		Name: "Rover",
	}
	idx.Add(char)

	id, ok := idx.Resolve("  rover  ")
	if !ok {
		t.Fatal("Resolve with whitespace: not found")
	}
	if id != 1 {
		t.Errorf("Resolve with whitespace: id=%d, want 1", id)
	}
}

func TestAliasDuplicateWinsLast(t *testing.T) {
	idx := NewAliasIndex()
	char1 := &Character{
		ID:   1,
		Name: "Rover",
	}
	char2 := &Character{
		ID:      2,
		Name:    "Rover",
		Aliases: []string{},
	}
	idx.Add(char1)
	idx.Add(char2)

	id, ok := idx.Resolve("rover")
	if !ok {
		t.Fatal("Resolve(rover): not found")
	}
	if id != 2 {
		t.Errorf("Resolve(rover): id=%d, want 2 (last wins)", id)
	}
}
