package orchestrator

import (
	"regexp"
	"strconv"
	"strings"
)

// snowflakeIDPattern matches a standalone 17-20 digit Discord snowflake.
// Leading/trailing word chars (letters/digits/underscore) are excluded so we
// don't match ids embedded inside larger tokens or numbers.
var snowflakeIDPattern = regexp.MustCompile(`(^|[^0-9A-Za-z_<])([0-9]{17,20})($|[^0-9A-Za-z_])`)

// existingMentionPattern matches a Discord mention so we can ignore digits
// that are already wrapped in valid mention syntax.
//
// Forms covered:
//   <@123>     user mention
//   <@!123>    legacy nickname mention
//   <@&123>    role mention
//   <#123>     channel mention
var existingMentionPattern = regexp.MustCompile(`<[@#][!&]?[0-9]{17,20}>`)

// scrubRawUserIDs replaces raw Discord snowflake ids belonging to knownIDs
// with valid `<@id>` mention syntax. Ids that already appear inside a Discord
// mention are left untouched, so explicit tags requested by the LLM survive.
//
// Unknown long-digit runs are left as-is to avoid corrupting unrelated numeric
// content (timestamps, version strings) - the plan scope is "known context
// user ids" only.
func scrubRawUserIDs(content string, knownIDs map[int64]bool) string {
	if content == "" || len(knownIDs) == 0 {
		return content
	}

	mentionSpans := existingMentionPattern.FindAllStringIndex(content, -1)
	insideMention := func(start, end int) bool {
		for _, span := range mentionSpans {
			if start >= span[0] && end <= span[1] {
				return true
			}
		}
		return false
	}

	return snowflakeIDPattern.ReplaceAllStringFunc(content, func(match string) string {
		loc := snowflakeIDPattern.FindStringSubmatchIndex(match)
		if loc == nil {
			return match
		}
		idStart := loc[4]
		idEnd := loc[5]
		idStr := match[idStart:idEnd]

		absStart := strings.Index(content, match)
		if absStart >= 0 && insideMention(absStart+idStart, absStart+idEnd) {
			return match
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || !knownIDs[id] {
			return match
		}

		return match[:idStart] + "<@" + idStr + ">" + match[idEnd:]
	})
}

// collectKnownUserIDs gathers user ids that are present in the conversation
// context so the leak guard knows which raw snowflakes to wrap into mentions.
func collectKnownUserIDs(triggeringUserID int64, history []map[string]string) map[int64]bool {
	ids := make(map[int64]bool, len(history)+1)
	if triggeringUserID > 0 {
		ids[triggeringUserID] = true
	}
	for _, msg := range history {
		extractIDsFromText(msg["content"], ids)
	}
	return ids
}

var labelIDPattern = regexp.MustCompile(`user id: ([0-9]{17,20})`)

// taggedIDPattern extracts the userid field from the
// <channel|thread|username|userid|timestamp|message> wire format used in
// allowed-channel and lore-anchor context blocks.
var taggedIDPattern = regexp.MustCompile(`<[^|<>]*\|[^|<>]*\|[^|<>]*\|([0-9]{17,20})\|`)

func extractIDsFromText(text string, into map[int64]bool) {
	for _, m := range labelIDPattern.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		if id, err := strconv.ParseInt(m[1], 10, 64); err == nil && id > 0 {
			into[id] = true
		}
	}
	for _, m := range taggedIDPattern.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		if id, err := strconv.ParseInt(m[1], 10, 64); err == nil && id > 0 {
			into[id] = true
		}
	}
}

// usernamePattern extracts username from "username (user id: N)" label format.
// Captures the username part (group 1) and the user id (group 2).
var usernamePattern = regexp.MustCompile(`^([a-zA-Z0-9._-]+)\s+\(user id: ([0-9]{17,20})\)`)

// taggedUsernamePattern extracts username from tagged format <channel|thread|username|userid|...>.
// Captures username (group 1) and userid (group 2).
var taggedUsernamePattern = regexp.MustCompile(`<[^|<>]*\|[^|<>]*\|([^|<>]+)\|([0-9]{17,20})\|`)

// collectKnownUserMap gathers both user ids and username->id mappings from conversation context.
// Returns (knownIDs map, usernameToID map).
func collectKnownUserMap(triggeringUserID int64, triggeringUserName string, history []map[string]string) (map[int64]bool, map[string]int64) {
	ids := make(map[int64]bool, len(history)+1)
	usernames := make(map[string]int64)

	if triggeringUserID > 0 {
		ids[triggeringUserID] = true
	}
	if triggeringUserName != "" {
		usernames[strings.ToLower(triggeringUserName)] = triggeringUserID
	}

	for _, msg := range history {
		extractIDsFromText(msg["content"], ids)
		extractUsernamesFromText(msg["content"], usernames)
	}

	return ids, usernames
}

// extractUsernamesFromText extracts username->id mappings from label and tagged formats.
func extractUsernamesFromText(text string, into map[string]int64) {
	// Extract from "username (user id: N)" labels
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		m := usernamePattern.FindStringSubmatch(line)
		if len(m) >= 3 {
			username := strings.ToLower(m[1])
			if id, err := strconv.ParseInt(m[2], 10, 64); err == nil && id > 0 {
				into[username] = id
			}
		}
	}

	// Extract from tagged format <channel|thread|username|userid|timestamp|message>
	for _, m := range taggedUsernamePattern.FindAllStringSubmatch(text, -1) {
		if len(m) >= 3 {
			username := strings.ToLower(m[1])
			if id, err := strconv.ParseInt(m[2], 10, 64); err == nil && id > 0 {
				into[username] = id
			}
		}
	}
}

// stopWords is a denylist of common English words to avoid false-positive username promotion.
var stopWords = map[string]bool{
	"the": true, "and": true, "or": true, "to": true, "at": true,
	"in": true, "is": true, "it": true, "you": true, "me": true,
	"we": true, "us": true, "him": true, "her": true, "a": true,
	"an": true, "be": true, "by": true,
}

// replacePlainUsernamesWithMentions converts plain username references to Discord mentions.
// Skips usernames that are:
// - Already inside mention syntax (<@id>, <@!id>, <@&id>, <#id>)
// - Inside the "username (user id: N)" label format
// - Inside code blocks (``` ... ```) or inline code (` ... `)
// - Too short (< 3 chars) or in the stop-word denylist
// Uses word boundaries and case-insensitive matching.
func replacePlainUsernamesWithMentions(content string, usernameToID map[string]int64) string {
	if content == "" || len(usernameToID) == 0 {
		return content
	}

	// Find all mention spans to skip
	mentionSpans := existingMentionPattern.FindAllStringIndex(content, -1)
	insideMention := func(start, end int) bool {
		for _, span := range mentionSpans {
			if start >= span[0] && end <= span[1] {
				return true
			}
		}
		return false
	}

	// Find all label spans "username (user id: N)" to skip
	labelSpans := regexp.MustCompile(`\b[a-zA-Z0-9._-]+\s+\(user id: [0-9]{17,20}\)`).FindAllStringIndex(content, -1)
	insideLabel := func(start, end int) bool {
		for _, span := range labelSpans {
			if start >= span[0] && end <= span[1] {
				return true
			}
		}
		return false
	}

	// Split on code fences and process only non-fenced parts
	fencePattern := regexp.MustCompile("```")
	parts := fencePattern.Split(content, -1)

	var result strings.Builder
	inFence := false

	for i, part := range parts {
		if inFence {
			// Inside a fenced code block, don't transform
			result.WriteString(part)
		} else {
			// Outside fenced blocks, mask inline code and transform
			part = processInlineCode(part, usernameToID, insideMention, insideLabel)
			result.WriteString(part)
		}

		// Toggle fence state (every other split is inside a fence)
		if i < len(parts)-1 {
			result.WriteString("```")
			inFence = !inFence
		}
	}

	return result.String()
}

// processInlineCode masks inline code spans, applies username replacement, then restores them.
func processInlineCode(text string, usernameToID map[string]int64, insideMention, insideLabel func(int, int) bool) string {
	inlineCodePattern := regexp.MustCompile("`[^`]+`")
	codeMasks := make(map[string]string)
	var codeCounter int

	masked := inlineCodePattern.ReplaceAllStringFunc(text, func(match string) string {
		placeholder := "\x00INLINECODE" + strconv.Itoa(codeCounter) + "\x00"
		codeMasks[placeholder] = match
		codeCounter++
		return placeholder
	})

	result := applyUsernameReplacements(masked, usernameToID, insideMention, insideLabel)

	for placeholder, original := range codeMasks {
		result = strings.ReplaceAll(result, placeholder, original)
	}

	return result
}

// applyUsernameReplacements performs the actual username->mention conversion.
func applyUsernameReplacements(text string, usernameToID map[string]int64, insideMention, insideLabel func(int, int) bool) string {
	for username, userID := range usernameToID {
		// Skip short usernames and stop words
		if len(username) < 3 || stopWords[username] {
			continue
		}

		// Build a case-insensitive word-boundary regex for this username
		// Escape special regex chars in the username
		escaped := regexp.QuoteMeta(username)
		pattern := regexp.MustCompile(`(?i)\b` + escaped + `\b`)

		text = pattern.ReplaceAllStringFunc(text, func(match string) string {
			// Find the position of this match in the current text
			// We need to check if it's inside a mention or label
			idx := strings.Index(text, match)
			if idx >= 0 && (insideMention(idx, idx+len(match)) || insideLabel(idx, idx+len(match))) {
				return match
			}
			return "<@" + strconv.FormatInt(userID, 10) + ">"
		})
	}
	return text
}

// scrubOutbound combines scrubRawUserIDs and replacePlainUsernamesWithMentions.
// Applies raw ID scrubbing first, then username replacement.
func scrubOutbound(content string, knownIDs map[int64]bool, usernameToID map[string]int64) string {
	// First pass: convert raw snowflake IDs to mentions
	content = scrubRawUserIDs(content, knownIDs)
	// Second pass: convert plain usernames to mentions
	content = replacePlainUsernamesWithMentions(content, usernameToID)
	return content
}
