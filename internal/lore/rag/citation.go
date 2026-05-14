package rag

import (
	"fmt"
	"strings"
)

type Citation struct {
	Title string
	URL   string
}

func FormatCitation(c Citation) string {
	return fmt.Sprintf("\"%s\" - %s", c.Title, c.URL)
}

func FormatMultiple(cs []Citation) string {
	if len(cs) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, c := range cs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("- " + FormatCitation(c))
	}
	return sb.String()
}
