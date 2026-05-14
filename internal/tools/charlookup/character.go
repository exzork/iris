package charlookup

import (
	"strings"
)

// Character represents a Wuthering Waves character with metadata and aliases.
type Character struct {
	ID      int64
	Name    string   // canonical name e.g. "Rover"
	Aliases []string // lower-cased aliases
	Element string   // e.g. "Spectro"
	Weapon  string   // e.g. "Sword"
	Rarity  string
	PageURL string // wiki URL
	Summary string // short Indonesian summary (fallback if RAG finds nothing)
}

// AliasIndex provides case-insensitive alias resolution to character IDs.
type AliasIndex struct {
	byAlias map[string]int64 // lower(alias) -> character ID
}

// NewAliasIndex creates a new empty alias index.
func NewAliasIndex() *AliasIndex {
	return &AliasIndex{
		byAlias: make(map[string]int64),
	}
}

// Add registers a character's name and aliases in the index.
// If multiple characters share the same alias, the last one wins.
func (a *AliasIndex) Add(char *Character) {
	if char == nil {
		return
	}

	// Add canonical name
	a.byAlias[strings.ToLower(char.Name)] = char.ID

	// Add all aliases
	for _, alias := range char.Aliases {
		a.byAlias[strings.ToLower(alias)] = char.ID
	}
}

// Resolve performs case-insensitive lookup of a query string.
// Returns the character ID and true if found, or 0 and false if not found.
func (a *AliasIndex) Resolve(query string) (int64, bool) {
	if query == "" {
		return 0, false
	}
	id, ok := a.byAlias[strings.ToLower(strings.TrimSpace(query))]
	return id, ok
}
