package chunker

import (
	"context"
	"fmt"

	"github.com/ETretyakov/rag-with-tables/internal/ingest"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
)

// Chunkify generates HYDE in parallel and converts every table row into a Chunk.
// fileID links all chunks to the uploaded file for later bulk deletion.
// If llm is nil, HYDE strings are empty and chunking continues uninterrupted.
// concurrency ≤ 0 defaults to 4.
func Chunkify(
	ctx context.Context,
	tables []ingest.NormalizedTable,
	llm provider.LLMProvider,
	concurrency int,
	fileID string,
) ([][]Chunk, error) {
	hydes, err := GenerateHYDEBatch(ctx, tables, llm, concurrency)
	if err != nil {
		return nil, fmt.Errorf("chunkify: hyde batch: %w", err)
	}

	result := make([][]Chunk, len(tables))
	for i, tbl := range tables {
		result[i] = MakeChunks(tbl, hydes[i], fileID)
	}
	return result, nil
}
