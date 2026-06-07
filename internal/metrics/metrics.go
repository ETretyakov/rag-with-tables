// Package metrics provides a simple in-process counter registry.
// All counters use atomic operations — safe for concurrent access.
package metrics

import (
	"sync/atomic"
)

// Registry holds all service-level counters.
type Registry struct {
	// Ingestion pipeline.
	IngestionFiles      atomic.Int64
	IngestionTables     atomic.Int64
	IngestionChunks     atomic.Int64
	IngestionDurationMs atomic.Int64

	// Query pipeline.
	QueryTotal      atomic.Int64
	QueryDurationMs atomic.Int64
	QuerySQLPath    atomic.Int64
	QueryTextPath   atomic.Int64

	// DuckDB table cache.
	CacheHits   atomic.Int64
	CacheMisses atomic.Int64
}

// Global is the singleton registry used across the whole process.
var Global = &Registry{}

// Snapshot returns all counters as a plain map suitable for JSON marshalling.
func (r *Registry) Snapshot() map[string]any {
	return map[string]any{
		"ingestion": map[string]any{
			"files_processed":    r.IngestionFiles.Load(),
			"tables_processed":   r.IngestionTables.Load(),
			"chunks_created":     r.IngestionChunks.Load(),
			"total_duration_ms":  r.IngestionDurationMs.Load(),
		},
		"query": map[string]any{
			"requests_total":    r.QueryTotal.Load(),
			"total_duration_ms": r.QueryDurationMs.Load(),
			"sql_path":          r.QuerySQLPath.Load(),
			"text_path":         r.QueryTextPath.Load(),
		},
		"duckdb_cache": map[string]any{
			"hits":   r.CacheHits.Load(),
			"misses": r.CacheMisses.Load(),
		},
	}
}
