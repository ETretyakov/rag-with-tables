package chunker

import "github.com/ETretyakov/rag-with-tables/internal/ingest/schema"

// ColumnInfo is a compact, JSON-serialisable column descriptor stored in Qdrant payload.
type ColumnInfo struct {
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	Type         string `json:"type"`
}

// ChunkMeta carries all metadata that ends up as a Qdrant point payload.
// Every field must be JSON-serialisable (Qdrant stores it as JSON).
type ChunkMeta struct {
	FileID     string       `json:"file_id"` // UUID of the parent uploaded file — used for bulk delete
	SourceFile string       `json:"source_file"`
	SheetName  string       `json:"sheet_name"`
	TableIndex int          `json:"table_index"`
	RowIndex   int          `json:"row_index"` // 0-based within the table
	S3Key      string       `json:"s3_key"`    // Parquet file location
	HYDE       string       `json:"hyde"`      // hypothetical questions for the table
	Summary    string       `json:"summary"`   // LLM-generated table description (set before normalisation)
	Schema     []ColumnInfo `json:"schema"`
}

// Chunk is a single vector-DB document: embeddable text + Qdrant payload.
type Chunk struct {
	Text string
	Meta ChunkMeta
}

// toColumnInfos converts schema.ColumnDef slice to the compact payload form.
func toColumnInfos(cols []schema.ColumnDef) []ColumnInfo {
	infos := make([]ColumnInfo, len(cols))
	for i, c := range cols {
		infos[i] = ColumnInfo{
			Name:         c.Name,
			OriginalName: c.OriginalName,
			Type:         string(c.Type),
		}
	}
	return infos
}
