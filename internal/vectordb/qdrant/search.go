package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ETretyakov/rag-with-tables/internal/chunker"
	"github.com/ETretyakov/rag-with-tables/internal/embedder"
)

// SearchHit is a single result from hybrid search.
type SearchHit struct {
	Score      float64
	Text       string
	S3Key      string
	FileID     string
	SourceFile string
	SheetName  string
	TableIndex int
	RowIndex   int
	Schema     []chunker.ColumnInfo
	HYDE       string
	Summary    string
}

// HybridSearch performs RRF fusion over dense and sparse prefetch queries.
// Returns up to topK hits ordered by fused score (descending).
func (c *Client) HybridSearch(
	ctx context.Context,
	dense []float32,
	sparse embedder.SparseVector,
	topK int,
) ([]SearchHit, error) {
	body := map[string]any{
		"prefetch": []map[string]any{
			{
				"query": dense,
				"using": "dense",
				"limit": topK * 2,
			},
			{
				"query": sparse,
				"using": "sparse",
				"limit": topK * 2,
			},
		},
		"query":        map[string]any{"fusion": "rrf"},
		"limit":        topK,
		"with_payload": true,
	}

	resp, err := c.do(ctx, http.MethodPost,
		"/collections/"+c.cfg.Collection+"/points/query", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant search HTTP %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Result struct {
			Points []struct {
				Score   float64        `json:"score"`
				Payload map[string]any `json:"payload"`
			} `json:"points"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("qdrant search decode: %w", err)
	}

	hits := make([]SearchHit, 0, len(result.Result.Points))
	for _, p := range result.Result.Points {
		hit := SearchHit{
			Score:      p.Score,
			Text:       strVal(p.Payload, "text"),
			S3Key:      strVal(p.Payload, "s3_key"),
			FileID:     strVal(p.Payload, "file_id"),
			SourceFile: strVal(p.Payload, "source_file"),
			SheetName:  strVal(p.Payload, "sheet_name"),
			HYDE:       strVal(p.Payload, "hyde"),
			Summary:    strVal(p.Payload, "summary"),
		}
		if v, ok := p.Payload["table_index"]; ok {
			if f, ok := v.(float64); ok {
				hit.TableIndex = int(f)
			}
		}
		if v, ok := p.Payload["row_index"]; ok {
			if f, ok := v.(float64); ok {
				hit.RowIndex = int(f)
			}
		}
		if v, ok := p.Payload["schema"]; ok {
			if raw, err := json.Marshal(v); err == nil {
				_ = json.Unmarshal(raw, &hit.Schema)
			}
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
