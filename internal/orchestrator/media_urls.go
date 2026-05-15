package orchestrator

import (
	"regexp"
	"strings"
)

// extractMediaURLs scans content for image/GIF URLs that should be rendered as
// Discord embeds (so the URL itself does not appear in the message text). It
// returns the cleaned text (URLs removed and surrounding whitespace tidied)
// and the list of media URLs in the order they appeared.
//
// Recognized: giphy.com, media.giphy.com, tenor.com, media.tenor.com,
// cdn.discordapp.com/stickers/*, and any URL ending in .gif, .png, .jpg, .jpeg, or .webp.
func extractMediaURLs(content string) (string, []string) {
	if content == "" {
		return content, nil
	}
	matches := mediaURLRegexp.FindAllString(content, -1)
	if len(matches) == 0 {
		return content, nil
	}
	cleaned := mediaURLRegexp.ReplaceAllString(content, "")
	cleaned = whitespaceCollapseRegexp.ReplaceAllString(cleaned, " ")
	cleaned = blankLineCollapseRegexp.ReplaceAllString(cleaned, "\n\n")
	cleaned = strings.TrimSpace(cleaned)

	seen := make(map[string]struct{}, len(matches))
	urls := make([]string, 0, len(matches))
	for _, m := range matches {
		m = strings.TrimRight(m, ".,;:)]'\"")
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		urls = append(urls, m)
	}
	return cleaned, urls
}

var (
	mediaURLRegexp           = regexp.MustCompile(`https?://[^\s<]*?(?:giphy\.com|tenor\.com|cdn\.discordapp\.com/stickers/|\.(?:gif|png|jpe?g|webp))[^\s<]*`)
	whitespaceCollapseRegexp = regexp.MustCompile(`[ \t]+`)
	blankLineCollapseRegexp  = regexp.MustCompile(`\n{3,}`)
)
