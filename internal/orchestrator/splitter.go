package orchestrator

import "strings"

const DiscordMessageLimit = 2000

func SplitMessage(content string, limit int) []string {
	if content == "" {
		return nil
	}
	if limit <= 0 {
		limit = DiscordMessageLimit
	}
	if len(content) <= limit {
		return []string{content}
	}

	chunks := []string{}
	remaining := content
	for len(remaining) > limit {
		cut := findCut(remaining, limit)
		chunks = append(chunks, remaining[:cut])
		remaining = remaining[cut:]
	}
	if len(remaining) > 0 {
		chunks = append(chunks, remaining)
	}
	return chunks
}

func findCut(s string, limit int) int {
	window := s[:limit]
	if idx := strings.LastIndex(window, "\n"); idx > 0 {
		return idx + 1
	}
	if idx := strings.LastIndex(window, " "); idx > 0 {
		return idx + 1
	}
	return limit
}
