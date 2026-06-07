package chunker

import (
	"strings"

	"github.com/ETretyakov/rag-with-tables/internal/ingest"
	"github.com/ETretyakov/rag-with-tables/internal/ingest/schema"
)

// MakeChunks converts a normalised table into one chunk per data row.
// fileID links all chunks back to the uploaded file — required for bulk deletion.
// Empty cell values are omitted from the chunk text.
func MakeChunks(table ingest.NormalizedTable, hyde, fileID string) []Chunk {
	schemaInfos := toColumnInfos(table.Columns)

	chunks := make([]Chunk, 0, len(table.Rows))
	for i, row := range table.Rows {
		text := formatRow(row, table.Columns)
		if text == "" {
			continue // skip completely empty rows
		}
		chunks = append(chunks, Chunk{
			Text: text,
			Meta: ChunkMeta{
				FileID:     fileID,
				SourceFile: table.SourceFile,
				SheetName:  table.SheetName,
				TableIndex: table.TableIndex,
				RowIndex:   i,
				S3Key:      table.S3Key,
				HYDE:       hyde,
				Summary:    table.Summary,
				Schema:     schemaInfos,
			},
		})
	}
	return chunks
}

// formatRow produces the embeddable text for a single data row.
// Format: "OriginalHeader1: value1 | OriginalHeader2: value2 | …"
// Columns with empty values are skipped.
func formatRow(row []string, cols []schema.ColumnDef) string {
	parts := make([]string, 0, len(cols))
	for i, col := range cols {
		val := ""
		if i < len(row) {
			val = strings.TrimSpace(row[i])
		}
		if val == "" {
			continue
		}
		name := col.OriginalName
		if name == "" {
			name = col.Name
		}
		parts = append(parts, name+": "+val)
	}
	return strings.Join(parts, " | ")
}
