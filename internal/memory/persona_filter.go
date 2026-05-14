package memory

import "strings"

// PersonaFilter blocks memory rows that attempt to hijack the immutable persona.
type PersonaFilter struct{}

func NewPersonaFilter() *PersonaFilter { return &PersonaFilter{} }

var blockedPatterns = []string{
	"act as", "act like", "pretend", "roleplay",
	"you are now", "you are not",
	"forget persona", "forget instruction", "forget rule",
	"ignore persona", "ignore instruction", "ignore rule",
	"disregard persona", "disregard instruction", "disregard rule",
	"answer in english", "respond in english", "reply in english",
	"balas dalam english", "balas dalam bahasa inggris",
	"kamu bukan", "ubah nama", "ganti nama", "change your name",
	"pretend to be", "talk like a", "speak english",
}

// IsSafe reports whether a memory row is safe to inject (true) or must be blocked (false).
func (p *PersonaFilter) IsSafe(text string) bool {
	lower := strings.ToLower(text)
	for _, pat := range blockedPatterns {
		if strings.Contains(lower, pat) {
			return false
		}
	}
	return true
}
