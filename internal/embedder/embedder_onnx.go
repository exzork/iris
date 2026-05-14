//go:build cgo

package embedder

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/daulet/tokenizers"
	ort "github.com/yalue/onnxruntime_go"
)

var (
	ortInitOnce sync.Once
	ortInitErr  error
)

func NewONNX(cfg ONNXConfig) (Embedder, error) {
	if cfg.ModelPath == "" {
		cfg.ModelPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/model.onnx"
	}
	if cfg.TokenizerPath == "" {
		cfg.TokenizerPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/tokenizer.json"
	}
	if cfg.MaxSeqLen == 0 {
		cfg.MaxSeqLen = 128
	}

	if _, err := os.Stat(cfg.ModelPath); err != nil {
		return nil, fmt.Errorf("model file not found at %s: %w", cfg.ModelPath, err)
	}
	if _, err := os.Stat(cfg.TokenizerPath); err != nil {
		return nil, fmt.Errorf("tokenizer file not found at %s: %w", cfg.TokenizerPath, err)
	}

	libPath := os.Getenv("IRIS_ONNXRUNTIME_LIB_PATH")
	if libPath == "" {
		libPath = "/usr/local/lib/libonnxruntime.so"
	}

	ort.SetSharedLibraryPath(libPath)

	ortInitOnce.Do(func() {
		ortInitErr = ort.InitializeEnvironment()
	})
	if ortInitErr != nil {
		return nil, fmt.Errorf("failed to initialize ort environment: %w", ortInitErr)
	}

	tokenizer, err := tokenizers.FromFile(cfg.TokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load tokenizer: %w", err)
	}

	session, err := ort.NewDynamicAdvancedSession(cfg.ModelPath, []string{"input_ids", "attention_mask", "token_type_ids"}, []string{"last_hidden_state"}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create ort session: %w", err)
	}

	return &onnxEmbedder{
		session:    session,
		tokenizer:  tokenizer,
		maxSeqLen:  cfg.MaxSeqLen,
		mu:         &sync.Mutex{},
	}, nil
}

type onnxEmbedder struct {
	session    *ort.DynamicAdvancedSession
	tokenizer  *tokenizers.Tokenizer
	maxSeqLen  int
	mu         *sync.Mutex
	closeOnce  sync.Once
	closed     bool
}

func (e *onnxEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, fmt.Errorf("embedder: closed")
	}

	encoding := e.tokenizer.EncodeWithOptions(text, true, tokenizers.WithReturnAttentionMask(), tokenizers.WithReturnTypeIDs())
	ids := encoding.IDs
	mask := encoding.AttentionMask
	typeIds := encoding.TypeIDs

	if len(ids) > e.maxSeqLen {
		ids = ids[:e.maxSeqLen]
		mask = mask[:e.maxSeqLen]
		typeIds = typeIds[:e.maxSeqLen]
	}

	padLen := e.maxSeqLen - len(ids)
	if padLen > 0 {
		ids = append(ids, make([]uint32, padLen)...)
		mask = append(mask, make([]uint32, padLen)...)
		typeIds = append(typeIds, make([]uint32, padLen)...)
	}

	inputIds := make([]int64, len(ids))
	attentionMask := make([]int64, len(mask))
	tokenTypeIds := make([]int64, len(typeIds))

	for i := range ids {
		inputIds[i] = int64(ids[i])
		attentionMask[i] = int64(mask[i])
		tokenTypeIds[i] = int64(typeIds[i])
	}

	inputShape := ort.Shape{1, int64(e.maxSeqLen)}

	inputIdsTensor, err := ort.NewTensor(inputShape, inputIds)
	if err != nil {
		return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
	}
	defer inputIdsTensor.Destroy()

	maskTensor, err := ort.NewTensor(inputShape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	typeTensor, err := ort.NewTensor(inputShape, tokenTypeIds)
	if err != nil {
		return nil, fmt.Errorf("failed to create token_type_ids tensor: %w", err)
	}
	defer typeTensor.Destroy()

	inputs := []ort.Value{inputIdsTensor, maskTensor, typeTensor}
	outputs := make([]ort.Value, 1)

	if err := e.session.Run(inputs, outputs); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}
	defer func() {
		for _, out := range outputs {
			if out != nil {
				out.Destroy()
			}
		}
	}()

	if len(outputs) == 0 || outputs[0] == nil {
		return nil, fmt.Errorf("no outputs from model")
	}

	lastHiddenState, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output type: %T", outputs[0])
	}

	embeddings := lastHiddenState.GetData()

	tokenEmbeddings := make([][]float32, e.maxSeqLen)
	for i := 0; i < e.maxSeqLen; i++ {
		tokenEmbeddings[i] = embeddings[i*DefaultDim : (i+1)*DefaultDim]
	}

	attentionMaskInt := make([]int, len(mask))
	for i, m := range mask {
		attentionMaskInt[i] = int(m)
	}

	pooled := MeanPool(tokenEmbeddings, attentionMaskInt)
	normalized := L2Normalize(pooled)

	return normalized, nil
}

func (e *onnxEmbedder) Dim() int {
	return DefaultDim
}

func (e *onnxEmbedder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var closeErr error
	e.closeOnce.Do(func() {
		if e.session != nil {
			e.session.Destroy()
		}
		e.closed = true
	})
	return closeErr
}
