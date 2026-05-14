//go:build !cgo

package embedder

import (
	"errors"
)

func NewONNX(cfg ONNXConfig) (Embedder, error) {
	return nil, errors.New("embedder: ONNX backend requires CGO_ENABLED=1; rebuild with CGO or use a FakeEmbedder")
}
