package ingest

import (
	"regexp"
	"strings"
)

var (
	reHTMLComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	reRefTag        = regexp.MustCompile(`(?si)<ref[^>]*?>.*?</ref>`)
	reSelfRefTag    = regexp.MustCompile(`(?i)<ref[^>]*?/>`)
	reHTMLTag       = regexp.MustCompile(`(?si)<[^>]+>`)
	reFileLink      = regexp.MustCompile(`(?i)\[\[(?:File|Image):[^\[\]]*?(?:\[\[[^\[\]]*?\]\][^\[\]]*?)*\]\]`)
	reExtLink       = regexp.MustCompile(`\[https?://\S+\s+([^\]]+)\]`)
	reExtLinkBare   = regexp.MustCompile(`\[https?://\S+\]`)
	reBoldItalic    = regexp.MustCompile(`'''+`)
	reItalic        = regexp.MustCompile(`''+`)
	reHeading       = regexp.MustCompile(`(?m)^(={1,6})\s*(.+?)\s*={1,6}\s*$`)
	reTableMarkup   = regexp.MustCompile(`(?m)^(\{\||\|\}|\|-|!|\|).*$`)
	reMagicWord     = regexp.MustCompile(`__[A-Z]+__`)
	reBlankLines    = regexp.MustCompile(`\n{3,}`)
	reTrailingSpace = regexp.MustCompile(`[ \t]+\n`)
)

// CleanWikitext strips MediaWiki markup so the embedder sees prose, not
// templates. Strips {{...}} (recursively), [[File:...]], <ref>, HTML tags,
// HTML comments, italic/bold markers, table markup, magic words, and
// extracts display text from [[Link|alias]] and [text](url) wikilinks.
func CleanWikitext(text string) string {
	if text == "" {
		return text
	}

	text = stripBalancedBraces(text)
	text = reHTMLComment.ReplaceAllString(text, "")
	text = reRefTag.ReplaceAllString(text, "")
	text = reSelfRefTag.ReplaceAllString(text, "")

	text = stripFileLinks(text)
	text = stripWikilinks(text)
	text = reExtLink.ReplaceAllString(text, "$1")
	text = reExtLinkBare.ReplaceAllString(text, "")
	text = reHTMLTag.ReplaceAllString(text, "")

	text = reHeading.ReplaceAllString(text, "$2")
	text = reTableMarkup.ReplaceAllString(text, "")
	text = reMagicWord.ReplaceAllString(text, "")
	text = reBoldItalic.ReplaceAllString(text, "")
	text = reItalic.ReplaceAllString(text, "")

	text = reTrailingSpace.ReplaceAllString(text, "\n")
	text = reBlankLines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

// stripBalancedBraces removes {{...}} runs, including nested templates,
// without using regex (regex cannot match balanced delimiters).
func stripBalancedBraces(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	i := 0
	for i < len(text) {
		if i+1 < len(text) && text[i] == '{' && text[i+1] == '{' {
			depth := 1
			j := i + 2
			for j < len(text) && depth > 0 {
				if j+1 < len(text) && text[j] == '{' && text[j+1] == '{' {
					depth++
					j += 2
					continue
				}
				if j+1 < len(text) && text[j] == '}' && text[j+1] == '}' {
					depth--
					j += 2
					continue
				}
				j++
			}
			if depth == 0 {
				i = j
				continue
			}
			break
		}
		out.WriteByte(text[i])
		i++
	}
	if i < len(text) {
		out.WriteString(text[i:])
	}
	return out.String()
}

func stripFileLinks(text string) string {
	for {
		next := reFileLink.ReplaceAllString(text, "")
		if next == text {
			return text
		}
		text = next
	}
}

func stripWikilinks(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	i := 0
	for i < len(text) {
		if i+1 < len(text) && text[i] == '[' && text[i+1] == '[' {
			depth := 1
			j := i + 2
			for j < len(text) && depth > 0 {
				if j+1 < len(text) && text[j] == '[' && text[j+1] == '[' {
					depth++
					j += 2
					continue
				}
				if j+1 < len(text) && text[j] == ']' && text[j+1] == ']' {
					depth--
					j += 2
					continue
				}
				j++
			}
			if depth == 0 {
				inner := text[i+2 : j-2]
				if pipe := strings.LastIndex(inner, "|"); pipe >= 0 {
					out.WriteString(inner[pipe+1:])
				} else {
					out.WriteString(inner)
				}
				i = j
				continue
			}
			break
		}
		out.WriteByte(text[i])
		i++
	}
	if i < len(text) {
		out.WriteString(text[i:])
	}
	return out.String()
}
