// Package qdrant provides a lightweight REST client for Qdrant vector database.
// Uses raw HTTP — no gRPC dependency required.
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ETretyakov/rag-with-tables/internal/chunker"
	"github.com/ETretyakov/rag-with-tables/internal/config"
	"github.com/ETretyakov/rag-with-tables/internal/embedder"
)

const (
	qdrantTimeout  = 30 * time.Second
	upsertPageSize = 50 // points per PUT /points request
)

// Client is a minimal Qdrant REST client.
type Client struct {
	cfg  config.QdrantConfig
	http *http.Client
	base string
}

func New(cfg config.QdrantConfig) *Client {
	scheme := "http"
	if cfg.UseTLS {
		scheme = "https"
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: qdrantTimeout},
		base: fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port),
	}
}

// ── Collection management ─────────────────────────────────────────────────────

// EnsureCollection creates the collection if it does not exist.
// The collection uses two named vector spaces:
//   - "dense"  — float32 cosine similarity (size = dims)
//   - "sparse" — BM25-style sparse vectors for hybrid search
func (c *Client) EnsureCollection(ctx context.Context, dims int) error {
	exists, err := c.collectionExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return c.createCollection(ctx, dims)
}

func (c *Client) collectionExists(ctx context.Context) (bool, error) {
	resp, err := c.do(ctx, http.MethodGet,
		"/collections/"+c.cfg.Collection, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func (c *Client) createCollection(ctx context.Context, dims int) error {
	body := map[string]any{
		"vectors": map[string]any{
			"dense": map[string]any{
				"size":     dims,
				"distance": "Cosine",
			},
		},
		"sparse_vectors": map[string]any{
			"sparse": map[string]any{},
		},
	}
	resp, err := c.do(ctx, http.MethodPut,
		"/collections/"+c.cfg.Collection, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant: create collection HTTP %d: %s", resp.StatusCode, raw)
	}
	return nil
}

// EnsurePayloadIndex creates a keyword index on field_name (idempotent).
// Used to enable fast filtering by s3_key when loading tables from S3.
func (c *Client) EnsurePayloadIndex(ctx context.Context, fieldName string) error {
	body := map[string]any{
		"field_name":   fieldName,
		"field_schema": "keyword",
	}
	resp, err := c.do(ctx, http.MethodPut,
		"/collections/"+c.cfg.Collection+"/index", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 200 = created, 409 = already exists — both are fine.
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusConflict {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant: create index HTTP %d: %s", resp.StatusCode, raw)
	}
	return nil
}

// ── Point upsert ──────────────────────────────────────────────────────────────

// point is the Qdrant point structure for upsert.
type point struct {
	ID      string         `json:"id"`
	Vector  pointVectors   `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type pointVectors struct {
	Dense  []float32              `json:"dense"`
	Sparse *embedder.SparseVector `json:"sparse,omitempty"`
}

// UpsertChunks upserts all chunks with their dense and sparse vectors.
// Point IDs are deterministic (UUID v5) — re-ingesting the same file
// updates existing points instead of creating duplicates.
func (c *Client) UpsertChunks(
	ctx context.Context,
	chunks []chunker.Chunk,
	dense [][]float32,
) error {
	points := make([]point, len(chunks))
	for i, ch := range chunks {
		sparse := embedder.BM25Sparse(ch.Text)
		points[i] = point{
			ID:      chunkID(ch),
			Vector:  pointVectors{Dense: dense[i], Sparse: &sparse},
			Payload: chunkPayload(ch),
		}
	}

	// Batch into pages of upsertPageSize.
	for start := 0; start < len(points); start += upsertPageSize {
		end := start + upsertPageSize
		if end > len(points) {
			end = len(points)
		}
		if err := c.upsertBatch(ctx, points[start:end]); err != nil {
			return fmt.Errorf("qdrant: upsert batch [%d:%d]: %w", start, end, err)
		}
	}
	return nil
}

func (c *Client) upsertBatch(ctx context.Context, pts []point) error {
	body := map[string]any{"points": pts}
	resp, err := c.do(ctx, http.MethodPut,
		"/collections/"+c.cfg.Collection+"/points", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, raw)
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// chunkID creates a deterministic UUID v5 from chunk coordinates.
// Same source_file + sheet_name + table_index + row_index → same ID,
// so re-ingestion is idempotent.
func chunkID(ch chunker.Chunk) string {
	key := fmt.Sprintf("%s|%s|%d|%d",
		ch.Meta.SourceFile, ch.Meta.SheetName,
		ch.Meta.TableIndex, ch.Meta.RowIndex)
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(key)).String()
}

func chunkPayload(ch chunker.Chunk) map[string]any {
	return map[string]any{
		"file_id":     ch.Meta.FileID, // indexed for bulk delete by file
		"source_file": ch.Meta.SourceFile,
		"sheet_name":  ch.Meta.SheetName,
		"table_index": ch.Meta.TableIndex,
		"row_index":   ch.Meta.RowIndex,
		"s3_key":      ch.Meta.S3Key,
		"hyde":        ch.Meta.HYDE,
		"summary":     ch.Meta.Summary,
		"schema":      ch.Meta.Schema,
		"text":        ch.Text,
	}
}

// DeleteByFileID removes all Qdrant points whose payload file_id matches.
// Called by DELETE /files/{id} to clean up indexed data for a specific upload.
func (c *Client) DeleteByFileID(ctx context.Context, fileID string) error {
	body := map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{
				{"key": "file_id", "match": map[string]any{"value": fileID}},
			},
		},
	}
	resp, err := c.do(ctx, http.MethodPost,
		"/collections/"+c.cfg.Collection+"/points/delete", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant delete by file_id HTTP %d: %s", resp.StatusCode, raw)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("qdrant: marshal: %w", err)
		}
		r = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return nil, fmt.Errorf("qdrant: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("api-key", c.cfg.APIKey)
	}

	return c.http.Do(req)
}
