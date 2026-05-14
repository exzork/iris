package embedder

import (
	"context"
	"math"
	"testing"
)

// TestFakeEmbedderDeterministic verifies that FakeEmbedder produces consistent output.
func TestFakeEmbedderDeterministic(t *testing.T) {
	fake := NewFakeEmbedder()

	text := "hello world"
	v1, err := fake.Embed(context.Background(), text)
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	v2, err := fake.Embed(context.Background(), text)
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	if len(v1) != len(v2) {
		t.Fatalf("vector lengths differ: %d vs %d", len(v1), len(v2))
	}

	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("vectors differ at index %d: %f vs %f", i, v1[i], v2[i])
		}
	}
}

// TestFakeEmbedderDim verifies output dimension is 384.
func TestFakeEmbedderDim(t *testing.T) {
	fake := NewFakeEmbedder()
	if fake.Dim() != 384 {
		t.Fatalf("expected Dim()=384, got %d", fake.Dim())
	}
}

// TestFakeEmbedderUnitNorm verifies vectors are L2-normalized.
func TestFakeEmbedderUnitNorm(t *testing.T) {
	fake := NewFakeEmbedder()
	v, err := fake.Embed(context.Background(), "test vector")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}

	norm := float32(0)
	for _, x := range v {
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))

	// Allow small floating-point error
	if math.Abs(float64(norm-1.0)) > 1e-5 {
		t.Fatalf("expected unit norm, got %f", norm)
	}
}

// TestMeanPoolingWithMask verifies that padding tokens are ignored.
func TestMeanPoolingWithMask(t *testing.T) {
	// 3 tokens, 2 dims each
	tokenEmbeddings := [][]float32{
		{1.0, 2.0},
		{3.0, 4.0},
		{0.0, 0.0}, // padding token
	}
	attentionMask := []int{1, 1, 0} // last token is padding

	result := MeanPool(tokenEmbeddings, attentionMask)

	// Expected: mean of first two tokens: [(1+3)/2, (2+4)/2] = [2, 3]
	expected := []float32{2.0, 3.0}
	if len(result) != len(expected) {
		t.Fatalf("expected length %d, got %d", len(expected), len(result))
	}

	for i := range result {
		if math.Abs(float64(result[i]-expected[i])) > 1e-5 {
			t.Fatalf("expected result[%d]=%f, got %f", i, expected[i], result[i])
		}
	}
}

// TestMeanPoolingAllMasked verifies behavior when all tokens are masked.
func TestMeanPoolingAllMasked(t *testing.T) {
	tokenEmbeddings := [][]float32{
		{1.0, 2.0},
		{3.0, 4.0},
	}
	attentionMask := []int{0, 0}

	result := MeanPool(tokenEmbeddings, attentionMask)

	// Should return zero vector
	for i, v := range result {
		if v != 0.0 {
			t.Fatalf("expected result[%d]=0, got %f", i, v)
		}
	}
}

// TestL2Normalize_UnitNorm verifies normalization produces unit norm.
func TestL2Normalize_UnitNorm(t *testing.T) {
	v := []float32{3.0, 4.0} // norm = 5
	normalized := L2Normalize(v)

	// Expected: [0.6, 0.8]
	expected := []float32{0.6, 0.8}
	for i := range normalized {
		if math.Abs(float64(normalized[i]-expected[i])) > 1e-5 {
			t.Fatalf("expected normalized[%d]=%f, got %f", i, expected[i], normalized[i])
		}
	}

	// Verify unit norm
	norm := float32(0)
	for _, x := range normalized {
		norm += x * x
	}
	norm = float32(math.Sqrt(float64(norm)))
	if math.Abs(float64(norm-1.0)) > 1e-5 {
		t.Fatalf("expected unit norm, got %f", norm)
	}
}

// TestL2Normalize_ZeroVector handles zero vector gracefully.
func TestL2Normalize_ZeroVector(t *testing.T) {
	v := []float32{0.0, 0.0}
	normalized := L2Normalize(v)

	// Should return zero vector (avoid division by zero)
	for i, x := range normalized {
		if !math.IsNaN(float64(x)) && x != 0.0 {
			t.Fatalf("expected normalized[%d]=0 or NaN, got %f", i, x)
		}
	}
}

// TestCosine_Identity_IsOne verifies cosine(v, v) = 1.
func TestCosine_Identity_IsOne(t *testing.T) {
	v := []float32{0.6, 0.8} // unit norm
	sim := Cosine(v, v)

	if math.Abs(float64(sim-1.0)) > 1e-5 {
		t.Fatalf("expected cosine(v,v)=1, got %f", sim)
	}
}

// TestCosine_Orthogonal_IsZero verifies cosine of orthogonal vectors is 0.
func TestCosine_Orthogonal_IsZero(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{0.0, 1.0}
	sim := Cosine(a, b)

	if math.Abs(float64(sim)) > 1e-5 {
		t.Fatalf("expected cosine(orthogonal)=0, got %f", sim)
	}
}

// TestCosine_Opposite_IsNegativeOne verifies cosine of opposite vectors is -1.
func TestCosine_Opposite_IsNegativeOne(t *testing.T) {
	a := []float32{1.0, 0.0}
	b := []float32{-1.0, 0.0}
	sim := Cosine(a, b)

	if math.Abs(float64(sim+1.0)) > 1e-5 {
		t.Fatalf("expected cosine(opposite)=-1, got %f", sim)
	}
}

// TestFakeEmbedderClose verifies Close() succeeds.
func TestFakeEmbedderClose(t *testing.T) {
	fake := NewFakeEmbedder()
	if err := fake.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}
