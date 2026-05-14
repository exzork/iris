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
