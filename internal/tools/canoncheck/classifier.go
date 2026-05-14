package canoncheck

import (
	"strings"

	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
)

// ClassifyContent inspects chunks vs claim text using simple heuristics:
// - No chunks: Unsupported
// - Fewer than MinChunks chunks above MinSupportScore: NeedsMoreSources
// - Chunks contain negation pattern contradicting claim keywords: Contradicted
// - Otherwise: Supported
func ClassifyContent(chunks []ragpkg.ScoredChunk, claim Claim, minChunks int, minSupportScore float64) Status {
	if len(chunks) == 0 {
		return StatusUnsupported
	}

	// Count chunks above threshold
	strongChunks := 0
	hasContradiction := false

	for _, chunk := range chunks {
		if chunk.Score >= minSupportScore {
			strongChunks++
		}

		// Check for negation patterns contradicting claim
		if detectContradiction(chunk.Content, claim.Text) {
			hasContradiction = true
		}
	}

	if hasContradiction {
		return StatusContradicted
	}

	if strongChunks < minChunks {
		return StatusNeedsMoreSources
	}

	return StatusSupported
}

// detectContradiction checks if chunk contains negation words near claim keywords
func detectContradiction(chunkContent, claimText string) bool {
	negationWords := []string{"tidak", "bukan", "not", "never"}
	claimKeywords := extractKeywords(claimText)

	chunkLower := strings.ToLower(chunkContent)

	for _, keyword := range claimKeywords {
		keywordLower := strings.ToLower(keyword)
		keywordIdx := strings.Index(chunkLower, keywordLower)
		if keywordIdx == -1 {
			continue
		}

		// Check for negation within 30 chars before keyword
		start := keywordIdx - 30
		if start < 0 {
			start = 0
		}
		context := chunkLower[start:keywordIdx]

		for _, negation := range negationWords {
			if strings.Contains(context, negation) {
				return true
			}
		}
	}

	return false
}

// extractKeywords extracts nouns/key terms from claim (simple heuristic)
func extractKeywords(claim string) []string {
	words := strings.Fields(claim)
	var keywords []string
	for _, word := range words {
		// Keep words longer than 3 chars, exclude common stop words
		if len(word) > 3 && !isStopWord(word) {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

// isStopWord checks if word is a common stop word
func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true, "is": true,
		"are": true, "was": true, "were": true, "be": true, "been": true,
	}
	return stopWords[strings.ToLower(word)]
}
