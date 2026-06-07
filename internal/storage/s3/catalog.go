package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
)

const catalogPrefix = "catalog/"

// ColumnSchema is a JSON-serialisable column descriptor stored in the catalog.
type ColumnSchema struct {
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	Type         string `json:"type"`
}

// TableMeta describes one table extracted from a file.
type TableMeta struct {
	TableIndex int            `json:"table_index"`
	SheetName  string         `json:"sheet_name,omitempty"`
	S3Key      string         `json:"s3_key"`
	RowCount   int            `json:"row_count"`
	Columns    []ColumnSchema `json:"columns"`
	Summary    string         `json:"summary,omitempty"`
}

// FileRecord is the persistent metadata stored in S3 for every indexed file.
// It is saved at catalog/{id}.json and used by GET /files and DELETE /files/{id}.
type FileRecord struct {
	ID          string      `json:"id"`
	Filename    string      `json:"filename"`
	UploadedAt  time.Time   `json:"uploaded_at"`
	TablesFound int         `json:"tables_found"`
	ChunksCount int         `json:"chunks_count"`
	S3Keys      []string    `json:"s3_keys"`              // Parquet locations, one per table
	Tables      []TableMeta `json:"tables,omitempty"`     // populated since v0.2
}

// SaveFileCatalog writes the FileRecord to S3 at catalog/{id}.json.
func (c *Client) SaveFileCatalog(ctx context.Context, rec FileRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal file record: %w", err)
	}
	return c.Upload(ctx, catalogPrefix+rec.ID+".json", data)
}

// GetFileCatalog retrieves the FileRecord for the given file ID.
func (c *Client) GetFileCatalog(ctx context.Context, id string) (*FileRecord, error) {
	data, err := c.Download(ctx, catalogPrefix+id+".json")
	if err != nil {
		return nil, fmt.Errorf("file %s not found: %w", id, err)
	}
	var rec FileRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal file record: %w", err)
	}
	return &rec, nil
}

// ListFileCatalog returns all FileRecords stored in the catalog prefix.
func (c *Client) ListFileCatalog(ctx context.Context) ([]FileRecord, error) {
	var records []FileRecord

	for obj := range c.mc.ListObjects(ctx, c.cfg.Bucket,
		minio.ListObjectsOptions{Prefix: catalogPrefix, Recursive: false}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list catalog: %w", obj.Err)
		}
		// Skip the prefix itself (minio may return it as an entry).
		if obj.Key == catalogPrefix {
			continue
		}
		data, err := c.Download(ctx, obj.Key)
		if err != nil {
			continue // best-effort: skip unreadable entries
		}
		var rec FileRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}

	return records, nil
}

// DeleteFileCatalog removes the catalog entry for the given file ID.
func (c *Client) DeleteFileCatalog(ctx context.Context, id string) error {
	return c.mc.RemoveObject(ctx, c.cfg.Bucket,
		catalogPrefix+id+".json", minio.RemoveObjectOptions{})
}

// DeleteParquetFiles removes Parquet files from S3 (best-effort, logs errors).
func (c *Client) DeleteParquetFiles(ctx context.Context, keys []string) error {
	var last error
	for _, key := range keys {
		if err := c.mc.RemoveObject(ctx, c.cfg.Bucket, key,
			minio.RemoveObjectOptions{}); err != nil {
			last = fmt.Errorf("delete %s: %w", key, err)
		}
	}
	return last
}
