package embedder

import (
	"context"
	"hash/fnv"
	"math"
)

const (
	DefaultDim = 384
)

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Dim() int
	Close() error
}

type ONNXConfig struct {
	ModelPath     string
	TokenizerPath string
	MaxSeqLen     int
	BatchSize     int
}

type FakeEmbedder struct{}

func NewFakeEmbedder() *FakeEmbedder {
	return &FakeEmbedder{}
}

func (f *FakeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	h := fnv.New64a()
	h.Write([]byte(text))
	seed := h.Sum64()

	v := make([]float32, DefaultDim)
	rng := newSeededRNG(seed)
	for i := range v {
		v[i] = rng.nextGaussian()
	}

	return L2Normalize(v), nil
}

func (f *FakeEmbedder) Dim() int {
	return DefaultDim
}

func (f *FakeEmbedder) Close() error {
	return nil
}

func MeanPool(tokenEmbeddings [][]float32, attentionMask []int) []float32 {
	if len(tokenEmbeddings) == 0 {
		return []float32{}
	}

	dim := len(tokenEmbeddings[0])
	result := make([]float32, dim)

	count := 0
	for i, mask := range attentionMask {
		if mask == 1 {
			for j := range result {
				result[j] += tokenEmbeddings[i][j]
			}
			count++
		}
	}

	if count > 0 {
		for i := range result {
			result[i] /= float32(count)
		}
	}

	return result
}

func L2Normalize(v []float32) []float32 {
	norm := float32(0)
	for _, x := range v {
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm == 0 {
		return v
	}

	result := make([]float32, len(v))
	for i, x := range v {
		result[i] = x / norm
	}
	return result
}

func Cosine(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	sim := float32(0)
	for i := range a {
		sim += a[i] * b[i]
	}
	return sim
}

type seededRNG struct {
	state uint64
}

func newSeededRNG(seed uint64) *seededRNG {
	return &seededRNG{state: seed}
}

func (r *seededRNG) nextGaussian() float32 {
	u1 := r.nextFloat()
	u2 := r.nextFloat()
	z0 := float32(math.Sqrt(-2*math.Log(float64(u1)))) * float32(math.Cos(2*math.Pi*float64(u2)))
	return z0
}

func (r *seededRNG) nextFloat() float32 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return float32((r.state >> 11) % (1 << 24)) / float32(1 << 24)
}
