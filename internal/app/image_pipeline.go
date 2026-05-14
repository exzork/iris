package app

import (
	"context"
	"strings"
)

type ImagePipeline struct {
	Gen ImagePort
}

type ImageResult struct {
	URL string
	Err error
}

func NewImagePipeline(gen ImagePort) *ImagePipeline { return &ImagePipeline{Gen: gen} }

func (p *ImagePipeline) Generate(ctx context.Context, prompt string) ImageResult {
	if p == nil || p.Gen == nil {
		return ImageResult{Err: ErrImageUnavailable}
	}
	url, err := p.Gen.Generate(ctx, prompt)
	if err != nil {
		return ImageResult{Err: err}
	}
	return ImageResult{URL: url}
}

func DetectIntent(query string) bool {
	q := strings.ToLower(query)
	triggers := []string{"buatkan gambar", "buat gambar", "generate image", "lukiskan", "render", "buat image"}
	for _, t := range triggers {
		if strings.Contains(q, t) {
			return true
		}
	}
	return false
}
