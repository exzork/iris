package charlookup

import (
	"context"
	"fmt"
	"strings"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
)

type Lookup struct {
	Store     CharacterStore
	Alias     *AliasIndex
	Retriever *ragpkg.Retriever
}

type LookupResult struct {
	Found     bool
	Character *Character
	Summary   string
	Citations []ragpkg.Citation
	Missing   string
}

func (l *Lookup) Find(ctx context.Context, query string) (*LookupResult, error) {
	if l.Store == nil || l.Alias == nil {
		return nil, fmt.Errorf("Lookup not properly initialized: Store and Alias required")
	}

	normalizedQuery := strings.TrimSpace(query)
	if normalizedQuery == "" {
		return &LookupResult{
			Found:   false,
			Missing: "Karakter tidak ditemukan. Silakan periksa wiki Wuthering Waves secara langsung: https://wutheringwaves.fandom.com/wiki/Special:Search",
		}, nil
	}

	charID, ok := l.Alias.Resolve(normalizedQuery)
	if !ok {
		return &LookupResult{
			Found:   false,
			Missing: fmt.Sprintf("Karakter `%s` belum terindeks. Silakan periksa wiki Wuthering Waves secara langsung: https://wutheringwaves.fandom.com/wiki/Special:Search", normalizedQuery),
		}, nil
	}

	char, err := l.Store.GetByID(ctx, charID)
	if err != nil {
		return &LookupResult{
			Found:   false,
			Missing: fmt.Sprintf("Karakter `%s` belum terindeks. Silakan periksa wiki Wuthering Waves secara langsung: https://wutheringwaves.fandom.com/wiki/Special:Search", normalizedQuery),
		}, nil
	}

	result := &LookupResult{
		Found:     true,
		Character: char,
		Summary:   char.Summary,
		Citations: []ragpkg.Citation{},
	}

	if l.Retriever != nil {
		scored, err := l.Retriever.Retrieve(ctx, char.Name, 5)
		if err == nil && len(scored) > 0 {
			var snippets []string
			citationMap := make(map[string]ragpkg.Citation)

			for _, sc := range scored {
				snippets = append(snippets, sc.Content)
				citationMap[sc.URL] = ragpkg.Citation{
					Title: sc.Title,
					URL:   sc.URL,
				}
			}

			if len(snippets) > 0 {
				result.Summary = char.Summary + "\n\n" + strings.Join(snippets, "\n\n")
			}

			for _, citation := range citationMap {
				result.Citations = append(result.Citations, citation)
			}
		}
	}

	return result, nil
}
