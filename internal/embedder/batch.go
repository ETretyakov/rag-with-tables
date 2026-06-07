// Package embedder handles batch embedding of chunks and sparse vector computation.
package embedder

import (
	"context"
	"fmt"

	"github.com/ETretyakov/rag-with-tables/internal/chunker"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
)

const DefaultBatchSize = 96 // VoyageAI allows up to 128; 96 leaves headroom

// EmbedAll embeds every chunk in batches and returns dense vectors in the same
// order as the input slice.
func EmbedAll(
	ctx context.Context,
	chunks []chunker.Chunk,
	ep provider.EmbeddingProvider,
	batchSize int,
) ([][]float32, error) {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	result := make([][]float32, len(chunks))

	for start := 0; start < len(chunks); start += batchSize {
		end := start + batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		texts := make([]string, end-start)
		for i, ch := range chunks[start:end] {
			texts[i] = ch.Text
		}

		vecs, err := ep.Embed(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d:%d]: %w", start, end, err)
		}
		if len(vecs) != len(texts) {
			return nil, fmt.Errorf("embed batch [%d:%d]: got %d vectors for %d texts",
				start, end, len(vecs), len(texts))
		}
		copy(result[start:end], vecs)
	}

	return result, nil
}
