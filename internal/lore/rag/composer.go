package rag

import "context"

type PromptContext struct {
	HasSupport bool
	Citations  []Citation
	Snippets   []string
}

type UnsupportedResponse struct {
	Message string
}

type Composer struct {
	Retriever *Retriever
	MinChunks int
}

func (c *Composer) Compose(ctx context.Context, query string) (*PromptContext, *UnsupportedResponse, error) {
	chunks, err := c.Retriever.Retrieve(ctx, query, 5)
	if err != nil {
		return nil, nil, err
	}

	if len(chunks) < c.MinChunks {
		return nil, &UnsupportedResponse{
			Message: "Belum ada data yang terindeks untuk pertanyaan ini. Silakan periksa wiki Wuthering Waves secara langsung: https://wutheringwaves.fandom.com/wiki/Special:Search",
		}, nil
	}

	snippets := make([]string, len(chunks))
	for i, chunk := range chunks {
		snippets[i] = chunk.Content
	}

	citationMap := make(map[string]Citation)
	citationOrder := make([]string, 0)

	for _, chunk := range chunks {
		if _, exists := citationMap[chunk.URL]; !exists {
			citationMap[chunk.URL] = Citation{Title: chunk.Title, URL: chunk.URL}
			citationOrder = append(citationOrder, chunk.URL)
		}
	}

	citations := make([]Citation, len(citationOrder))
	for i, url := range citationOrder {
		citations[i] = citationMap[url]
	}

	return &PromptContext{
		HasSupport: true,
		Citations:  citations,
		Snippets:   snippets,
	}, nil, nil
}
