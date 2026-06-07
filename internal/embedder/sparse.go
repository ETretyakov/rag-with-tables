package embedder

import (
	"hash/fnv"
	"strings"
	"unicode"
)

// maxSparseIndex is the hash modulus — gives ~1M distinct term buckets.
// Collisions are rare and acceptable for BM25-style sparse retrieval.
const maxSparseIndex = 1 << 20

// SparseVector is the Qdrant sparse vector format (indices + values, both same length).
type SparseVector struct {
	Indices []uint32  `json:"indices"`
	Values  []float32 `json:"values"`
}

// BM25Sparse computes a TF-based sparse vector for the text.
//
// Algorithm:
//  1. Tokenise: lowercase + split on non-letter/non-digit runes, drop tokens < 2 chars.
//  2. Compute normalised term frequency (TF = count / total).
//  3. Map each token to an index via FNV-32a % maxSparseIndex (no global vocab needed).
//
// This approximates sparse retrieval without requiring a corpus-wide IDF table.
// When combined with dense vectors via Qdrant RRF, it significantly improves recall
// on exact keyword matches (column names, codes, dates, etc.).
func BM25Sparse(text string) SparseVector {
	tf := termFreq(text)
	if len(tf) == 0 {
		return SparseVector{}
	}

	total := 0
	for _, c := range tf {
		total += c
	}

	indices := make([]uint32, 0, len(tf))
	values := make([]float32, 0, len(tf))

	for term, count := range tf {
		indices = append(indices, hashTerm(term))
		values = append(values, float32(count)/float32(total))
	}

	return SparseVector{Indices: indices, Values: values}
}

func termFreq(text string) map[string]int {
	freq := make(map[string]int)
	var buf strings.Builder

	flush := func() {
		if buf.Len() >= 2 {
			freq[buf.String()]++
		}
		buf.Reset()
	}

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(unicode.ToLower(r))
		} else {
			flush()
		}
	}
	flush()

	return freq
}

func hashTerm(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32() % maxSparseIndex
}
