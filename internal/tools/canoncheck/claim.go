package canoncheck

import ragpkg "github.com/eko/iris-bot/internal/lore/rag"

type Claim struct {
	Text  string
	Query string
}

type Verdict struct {
	Status     Status
	Confidence float64
	Citations  []ragpkg.Citation
	Snippets   []string
	Reason     string
}
