package memory

import (
	"strings"
	"unicode/utf8"
)

// Gate decides whether a message is worth saving to long-term memory.
// It rejects raw chat, pure questions, commands, and lore-only queries.
// It accepts user-stated preferences, self-disclosure, and explicit remember commands.
type Gate struct{}

func NewGate() *Gate { return &Gate{} }

// Decision reports whether text should be saved and, if skipped, why.
type Decision struct {
	Save   bool
	Reason string
}

// Preference markers (lowercase contains match).
var preferenceMarkers = []string{
	// Indonesian
	"aku suka", "aku tidak suka", "panggil aku", "panggil saya",
	"tolong selalu", "tolong jangan", "biasanya aku", "preferensi ku",
	"preferensi saya",
	// English
	"i prefer", "i like", "i don't like", "please always", "please never",
	"call me", "remember that", "remember this",
}

var rememberCommands = []string{
	"ingat ini", "simpan", "catat", "remember",
}

var selfDisclosure = []string{
	"nama saya", "nama ku", "nama aku", "my name is", "i am from",
	"saya dari", "timezone", "waktu saya", "i'm from",
}

var chatterPhrases = map[string]bool{
	"haha": true, "hahaha": true, "ok": true, "oke": true, "okay": true,
	"lol": true, "thanks": true, "thank you": true, "makasih": true,
	"terima kasih": true, "yes": true, "no": true, "ya": true, "tidak": true,
}

var loreKeywords = []string{
	"rover", "echo", "resonator", "wuthering waves", "wuwa", "fandom",
	"jinhsi", "encore", "calcharo", "yinlin",
}

// Decide classifies text for memory saving.
func (g *Gate) Decide(text string) Decision {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)

	if trimmed == "" {
		return Decision{Save: false, Reason: "empty"}
	}

	// Tool/command prefix
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "!") {
		return Decision{Save: false, Reason: "command"}
	}

	// Very short chatter
	if utf8.RuneCountInString(trimmed) < 10 {
		return Decision{Save: false, Reason: "too_short"}
	}
	if chatterPhrases[lower] {
		return Decision{Save: false, Reason: "chatter"}
	}

	// Preference / remember / self-disclosure override any question form.
	if containsAny(lower, preferenceMarkers) {
		return Decision{Save: true, Reason: "preference"}
	}
	if containsAny(lower, rememberCommands) {
		return Decision{Save: true, Reason: "remember_command"}
	}
	if containsAny(lower, selfDisclosure) {
		return Decision{Save: true, Reason: "self_disclosure"}
	}

	// Pure question without preference marker -> skip
	if strings.HasSuffix(trimmed, "?") {
		return Decision{Save: false, Reason: "question"}
	}

	// Pure lore chatter without personal context -> skip
	if containsAny(lower, loreKeywords) {
		return Decision{Save: false, Reason: "lore_only"}
	}

	return Decision{Save: false, Reason: "no_signal"}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
