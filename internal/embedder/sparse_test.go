package embedder_test

import (
	"testing"

	"github.com/ETretyakov/rag-with-tables/internal/embedder"
)

func TestBM25Sparse_BasicTokenization(t *testing.T) {
	sv := embedder.BM25Sparse("Name: Alice | Age: 30 | City: Moscow")

	if len(sv.Indices) == 0 {
		t.Fatal("expected non-empty sparse vector")
	}
	if len(sv.Indices) != len(sv.Values) {
		t.Fatalf("indices/values length mismatch: %d vs %d", len(sv.Indices), len(sv.Values))
	}

	// Weights should sum to ~1.0 (normalised TF).
	sum := float32(0)
	for _, v := range sv.Values {
		sum += v
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("weights sum = %f, want ~1.0", sum)
	}
}

func TestBM25Sparse_EmptyText(t *testing.T) {
	sv := embedder.BM25Sparse("")
	if len(sv.Indices) != 0 {
		t.Errorf("empty text should produce empty sparse vector, got %d terms", len(sv.Indices))
	}
}

func TestBM25Sparse_ShortTokensDropped(t *testing.T) {
	// Single-char tokens like "a" or "1" should be dropped.
	sv1 := embedder.BM25Sparse("a b c")
	sv2 := embedder.BM25Sparse("foo bar baz")
	if len(sv1.Indices) >= len(sv2.Indices) {
		t.Errorf("short tokens should produce fewer/no terms: got %d vs %d",
			len(sv1.Indices), len(sv2.Indices))
	}
}

func TestBM25Sparse_Deterministic(t *testing.T) {
	text := "Product: Widget | Price: 9.99 | Category: Electronics"
	sv1 := embedder.BM25Sparse(text)
	sv2 := embedder.BM25Sparse(text)

	if len(sv1.Indices) != len(sv2.Indices) {
		t.Fatalf("non-deterministic: got %d and %d indices", len(sv1.Indices), len(sv2.Indices))
	}
	// Same terms → same indices exist (order may differ due to map iteration)
	set1 := make(map[uint32]float32)
	for i, idx := range sv1.Indices {
		set1[idx] = sv1.Values[i]
	}
	for i, idx := range sv2.Indices {
		if v, ok := set1[idx]; !ok || abs32(v-sv2.Values[i]) > 1e-6 {
			t.Errorf("index %d: value mismatch or missing in first run", idx)
		}
	}
}

func TestBM25Sparse_RepeatTokensHigherWeight(t *testing.T) {
	// "name" appears 3× → should have higher weight than "alice" (1×).
	sv := embedder.BM25Sparse("name: alice | name: bob | name: charlie")

	nameIdx := hashFor("name")
	aliceIdx := hashFor("alice")

	var nameWeight, aliceWeight float32
	for i, idx := range sv.Indices {
		switch idx {
		case nameIdx:
			nameWeight = sv.Values[i]
		case aliceIdx:
			aliceWeight = sv.Values[i]
		}
	}

	if nameWeight <= aliceWeight {
		t.Errorf("'name' (3×) should have higher TF than 'alice' (1×): %f vs %f",
			nameWeight, aliceWeight)
	}
}

// hashFor replicates the internal hash so we can look up specific terms in tests.
func hashFor(s string) uint32 {
	// We know BM25Sparse uses FNV32a % 1<<20.
	// Re-implement here to keep the test self-contained.
	var h uint32 = 2166136261
	for _, b := range []byte(s) {
		h ^= uint32(b)
		h *= 16777619
	}
	return h % (1 << 20)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
