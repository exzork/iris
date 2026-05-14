//go:build embed_real

package embedder

import (
	"context"
	"math"
	"os"
	"sync"
	"testing"
)

func TestONNXIntegration_Dim384(t *testing.T) {
	modelPath := os.Getenv("IRIS_EMBED_MODEL_PATH")
	tokenizerPath := os.Getenv("IRIS_EMBED_TOKENIZER_PATH")

	if modelPath == "" {
		modelPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/model.onnx"
	}
	if tokenizerPath == "" {
		tokenizerPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/tokenizer.json"
	}

	cfg := ONNXConfig{
		ModelPath:     modelPath,
		TokenizerPath: tokenizerPath,
		MaxSeqLen:     128,
		BatchSize:     32,
	}

	emb, err := NewONNX(cfg)
	if err != nil {
		t.Fatalf("NewONNX failed: %v", err)
	}
	defer emb.Close()

	if emb.Dim() != 384 {
		t.Fatalf("expected Dim()=384, got %d", emb.Dim())
	}
}

func TestONNXIntegration_ParaphraseHigherThanUnrelated(t *testing.T) {
	modelPath := os.Getenv("IRIS_EMBED_MODEL_PATH")
	tokenizerPath := os.Getenv("IRIS_EMBED_TOKENIZER_PATH")

	if modelPath == "" {
		modelPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/model.onnx"
	}
	if tokenizerPath == "" {
		tokenizerPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/tokenizer.json"
	}

	cfg := ONNXConfig{
		ModelPath:     modelPath,
		TokenizerPath: tokenizerPath,
		MaxSeqLen:     128,
		BatchSize:     32,
	}

	emb, err := NewONNX(cfg)
	if err != nil {
		t.Fatalf("NewONNX failed: %v", err)
	}
	defer emb.Close()

	ctx := context.Background()

	v1, err := emb.Embed(ctx, "I'm tired")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	v2, err := emb.Embed(ctx, "I need sleep")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	v3, err := emb.Embed(ctx, "pizza recipe")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	simRelevant := Cosine(v1, v2)
	simUnrelated := Cosine(v1, v3)

	if simRelevant <= simUnrelated {
		t.Fatalf("expected sim(tired, sleep)=%f > sim(tired, pizza)=%f", simRelevant, simUnrelated)
	}
}

func TestONNXIntegration_Concurrent_RaceFree(t *testing.T) {
	modelPath := os.Getenv("IRIS_EMBED_MODEL_PATH")
	tokenizerPath := os.Getenv("IRIS_EMBED_TOKENIZER_PATH")

	if modelPath == "" {
		modelPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/model.onnx"
	}
	if tokenizerPath == "" {
		tokenizerPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/tokenizer.json"
	}

	cfg := ONNXConfig{
		ModelPath:     modelPath,
		TokenizerPath: tokenizerPath,
		MaxSeqLen:     128,
		BatchSize:     32,
	}

	emb, err := NewONNX(cfg)
	if err != nil {
		t.Fatalf("NewONNX failed: %v", err)
	}
	defer emb.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 8
	textsPerGoroutine := 5

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < textsPerGoroutine; i++ {
				text := "test message " + string(rune(id*textsPerGoroutine+i))
				v, err := emb.Embed(ctx, text)
				if err != nil {
					t.Errorf("Embed failed: %v", err)
					return
				}

				if len(v) != 384 {
					t.Errorf("expected len=384, got %d", len(v))
					return
				}

				norm := float32(0)
				for _, x := range v {
					norm += x * x
				}
				norm = float32(math.Sqrt(float64(norm)))
				if math.Abs(float64(norm-1.0)) > 1e-5 {
					t.Errorf("expected unit norm, got %f", norm)
					return
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestONNXEmbedder_CloseIdempotent(t *testing.T) {
	modelPath := os.Getenv("IRIS_EMBED_MODEL_PATH")
	tokenizerPath := os.Getenv("IRIS_EMBED_TOKENIZER_PATH")

	if modelPath == "" {
		modelPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/model.onnx"
	}
	if tokenizerPath == "" {
		tokenizerPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/tokenizer.json"
	}

	cfg := ONNXConfig{
		ModelPath:     modelPath,
		TokenizerPath: tokenizerPath,
		MaxSeqLen:     128,
		BatchSize:     32,
	}

	emb, err := NewONNX(cfg)
	if err != nil {
		t.Fatalf("NewONNX failed: %v", err)
	}

	// Call Close twice sequentially
	err1 := emb.Close()
	if err1 != nil {
		t.Fatalf("first Close() returned error: %v", err1)
	}

	err2 := emb.Close()
	if err2 != nil {
		t.Fatalf("second Close() returned error: %v", err2)
	}
}

func TestONNXEmbedder_EmbedAfterCloseReturnsError(t *testing.T) {
	modelPath := os.Getenv("IRIS_EMBED_MODEL_PATH")
	tokenizerPath := os.Getenv("IRIS_EMBED_TOKENIZER_PATH")

	if modelPath == "" {
		modelPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/model.onnx"
	}
	if tokenizerPath == "" {
		tokenizerPath = "/opt/iris-models/paraphrase-MiniLM-L3-v2/tokenizer.json"
	}

	cfg := ONNXConfig{
		ModelPath:     modelPath,
		TokenizerPath: tokenizerPath,
		MaxSeqLen:     128,
		BatchSize:     32,
	}

	emb, err := NewONNX(cfg)
	if err != nil {
		t.Fatalf("NewONNX failed: %v", err)
	}

	// Close the embedder
	if err := emb.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Try to Embed after Close
	ctx := context.Background()
	_, err = emb.Embed(ctx, "test message")
	if err == nil {
		t.Fatalf("expected Embed() to return error after Close(), got nil")
	}
}
