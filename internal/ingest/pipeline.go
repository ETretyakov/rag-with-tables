package ingest

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ETretyakov/rag-with-tables/internal/ingest/schema"
	duckstore "github.com/ETretyakov/rag-with-tables/internal/storage/duckdb"
	"github.com/ETretyakov/rag-with-tables/internal/storage/s3"
)

// Process converts a single RawTable into a NormalizedTable:
//  1. Normalize headers (snake_case, dedup, fill empty).
//  2. Infer column types from data rows.
//  3. Export to Parquet via in-memory DuckDB.
//  4. Upload Parquet to S3; attach the resulting key to NormalizedTable.
func Process(ctx context.Context, raw RawTable, s3c *s3.Client) (NormalizedTable, error) {
	cols := schema.NormalizeHeaders(raw.Headers)
	cols = schema.InferColumnTypes(cols, raw.Rows)

	parquet, err := duckstore.ToParquet(ctx, cols, raw.Rows)
	if err != nil {
		return NormalizedTable{}, fmt.Errorf("to parquet [%s/%s]: %w",
			raw.SheetName, raw.SourceFile, err)
	}

	s3Key := fmt.Sprintf("tables/%s.parquet", uuid.New().String())
	if err := s3c.Upload(ctx, s3Key, parquet); err != nil {
		return NormalizedTable{}, fmt.Errorf("upload [%s]: %w", s3Key, err)
	}

	return NormalizedTable{
		RawTable: raw,
		Columns:  cols,
		S3Key:    s3Key,
		RowCount: len(raw.Rows),
	}, nil
}

// ProcessAll runs Process for each table concurrently (up to concurrency goroutines).
// Results are returned in the same order as the input slice.
func ProcessAll(ctx context.Context, tables []RawTable, s3c *s3.Client, concurrency int) ([]NormalizedTable, error) {
	if concurrency <= 0 {
		concurrency = 4
	}

	type result struct {
		idx int
		tbl NormalizedTable
		err error
	}

	sem := make(chan struct{}, concurrency)
	ch := make(chan result, len(tables))

	for i, t := range tables {
		sem <- struct{}{}
		go func(idx int, raw RawTable) {
			defer func() { <-sem }()
			tbl, err := Process(ctx, raw, s3c)
			ch <- result{idx: idx, tbl: tbl, err: err}
		}(i, t)
	}

	out := make([]NormalizedTable, len(tables))
	for range tables {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		out[r.idx] = r.tbl
	}
	return out, nil
}
