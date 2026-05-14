package ingest

import "strings"

type Chunker struct {
	MaxChars int
	Overlap  int
}

type Chunk struct {
	Index   int
	Content string
	PageID  int64
	PageURL string
	Title   string
}

func NewChunker(maxChars, overlap int) *Chunker {
	if maxChars <= 0 {
		maxChars = 1000
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxChars {
		overlap = maxChars / 4
	}
	return &Chunker{MaxChars: maxChars, Overlap: overlap}
}

func (c *Chunker) Chunk(page *Page) []Chunk {
	if page == nil {
		return nil
	}
	text := strings.TrimSpace(page.Wikitext)
	if text == "" {
		return nil
	}

	maxChars := c.MaxChars
	if maxChars <= 0 {
		maxChars = 1000
	}
	overlap := c.Overlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxChars {
		overlap = maxChars / 4
	}

	base := splitByBoundaries(text, maxChars)
	if len(base) == 0 {
		return nil
	}

	withOverlap := applyOverlap(base, overlap)
	chunks := make([]Chunk, 0, len(withOverlap))
	for _, content := range withOverlap {
		if strings.TrimSpace(content) == "" {
			continue
		}
		chunks = append(chunks, Chunk{
			Index:   len(chunks),
			Content: content,
			PageID:  page.ID,
			PageURL: page.URL,
			Title:   page.Title,
		})
	}
	return chunks
}

func splitByBoundaries(text string, maxChars int) []string {
	paragraphs := strings.Split(text, "\n\n")
	chunks := make([]string, 0)
	current := ""

	flush := func() {
		trimmed := strings.TrimSpace(current)
		if trimmed != "" {
			chunks = append(chunks, trimmed)
		}
		current = ""
	}

	for _, para := range paragraphs {
		p := strings.TrimSpace(para)
		if p == "" {
			continue
		}
		if len(p) > maxChars {
			if current != "" {
				flush()
			}
			chunks = append(chunks, splitLongSegment(p, maxChars)...)
			continue
		}

		candidate := p
		if current != "" {
			candidate = current + "\n\n" + p
		}
		if len(candidate) <= maxChars {
			current = candidate
			continue
		}
		flush()
		current = p
	}
	if current != "" {
		flush()
	}
	return chunks
}

func splitLongSegment(text string, maxChars int) []string {
	sentences := splitSentences(text)
	chunks := make([]string, 0)
	current := ""

	flush := func() {
		trimmed := strings.TrimSpace(current)
		if trimmed != "" {
			chunks = append(chunks, trimmed)
		}
		current = ""
	}

	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if len(s) > maxChars {
			if current != "" {
				flush()
			}
			chunks = append(chunks, hardSplit(s, maxChars)...)
			continue
		}

		candidate := s
		if current != "" {
			candidate = current + " " + s
		}
		if len(candidate) <= maxChars {
			current = candidate
			continue
		}
		flush()
		current = s
	}
	if current != "" {
		flush()
	}
	return chunks
}

func splitSentences(text string) []string {
	parts := strings.Split(text, ".")
	out := make([]string, 0, len(parts))
	for idx, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if idx < len(parts)-1 {
			p += "."
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

func hardSplit(text string, maxChars int) []string {
	if maxChars <= 0 {
		return []string{text}
	}
	chunks := make([]string, 0)
	remaining := text
	for len(remaining) > maxChars {
		chunks = append(chunks, strings.TrimSpace(remaining[:maxChars]))
		remaining = remaining[maxChars:]
	}
	if strings.TrimSpace(remaining) != "" {
		chunks = append(chunks, strings.TrimSpace(remaining))
	}
	return chunks
}

func applyOverlap(chunks []string, overlap int) []string {
	if overlap <= 0 || len(chunks) <= 1 {
		return chunks
	}
	out := make([]string, len(chunks))
	copy(out, chunks)
	for i := 1; i < len(out); i++ {
		prev := out[i-1]
		tail := prev
		if len(tail) > overlap {
			tail = tail[len(tail)-overlap:]
		}
		if tail == "" {
			continue
		}
		out[i] = tail + out[i]
	}
	return out
}
