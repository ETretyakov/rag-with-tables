package chunker_test

import (
	"strings"
	"testing"

	"github.com/ETretyakov/rag-with-tables/internal/chunker"
	"github.com/ETretyakov/rag-with-tables/internal/ingest"
	"github.com/ETretyakov/rag-with-tables/internal/ingest/schema"
)

func makeTable(headers, originalHeaders []string, rows [][]string, s3key string) ingest.NormalizedTable {
	cols := make([]schema.ColumnDef, len(headers))
	for i, h := range headers {
		orig := h
		if i < len(originalHeaders) {
			orig = originalHeaders[i]
		}
		cols[i] = schema.ColumnDef{
			Index:        i,
			Name:         h,
			OriginalName: orig,
			Type:         schema.TypeVarchar,
		}
	}
	return ingest.NormalizedTable{
		RawTable: ingest.RawTable{
			SourceFile: "test.xlsx",
			SheetName:  "Sheet1",
			TableIndex: 0,
			Headers:    headers,
			Rows:       rows,
		},
		Columns:  cols,
		S3Key:    s3key,
		RowCount: len(rows),
	}
}

func TestMakeChunks_BasicFormatting(t *testing.T) {
	tbl := makeTable(
		[]string{"id", "name", "city"},
		[]string{"ID", "Name", "City"},
		[][]string{
			{"1", "Alice", "Moscow"},
			{"2", "Bob", "SPb"},
		},
		"tables/abc.parquet",
	)

	chunks := chunker.MakeChunks(tbl, "some hyde", "test-file-id")

	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(chunks))
	}

	want0 := "ID: 1 | Name: Alice | City: Moscow"
	if chunks[0].Text != want0 {
		t.Errorf("chunk[0].Text: got %q, want %q", chunks[0].Text, want0)
	}
}

func TestMakeChunks_EmptyValuesSkipped(t *testing.T) {
	tbl := makeTable(
		[]string{"a", "b", "c"},
		[]string{"A", "B", "C"},
		[][]string{
			{"1", "", "3"}, // B is empty → omitted
			{"", "", ""},   // all empty → chunk skipped
		},
		"",
	)

	chunks := chunker.MakeChunks(tbl, "", "test-file-id")

	// Second row is fully empty, should be skipped.
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk (empty row skipped), got %d", len(chunks))
	}
	if strings.Contains(chunks[0].Text, "B:") {
		t.Errorf("empty column B should not appear in chunk text: %q", chunks[0].Text)
	}
	want := "A: 1 | C: 3"
	if chunks[0].Text != want {
		t.Errorf("got %q, want %q", chunks[0].Text, want)
	}
}

func TestMakeChunks_MetaFields(t *testing.T) {
	tbl := makeTable(
		[]string{"x"},
		[]string{"X"},
		[][]string{{"hello"}},
		"tables/123.parquet",
	)
	const hydeText = "What values does this table have?"

	chunks := chunker.MakeChunks(tbl, hydeText, "test-file-id")

	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	m := chunks[0].Meta
	if m.SourceFile != "test.xlsx" {
		t.Errorf("SourceFile: %s", m.SourceFile)
	}
	if m.SheetName != "Sheet1" {
		t.Errorf("SheetName: %s", m.SheetName)
	}
	if m.RowIndex != 0 {
		t.Errorf("RowIndex: %d", m.RowIndex)
	}
	if m.S3Key != "tables/123.parquet" {
		t.Errorf("S3Key: %s", m.S3Key)
	}
	if m.HYDE != hydeText {
		t.Errorf("HYDE: %s", m.HYDE)
	}
	if len(m.Schema) != 1 || m.Schema[0].Name != "x" {
		t.Errorf("Schema: %v", m.Schema)
	}
}

func TestMakeChunks_RowIndexPreservedAfterSkip(t *testing.T) {
	// Row 0 is empty (skipped), row 1 has data → RowIndex should be 1.
	tbl := makeTable(
		[]string{"v"},
		[]string{"V"},
		[][]string{
			{""},
			{"hello"},
		},
		"",
	)

	chunks := chunker.MakeChunks(tbl, "", "test-file-id")
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Meta.RowIndex != 1 {
		t.Errorf("RowIndex: got %d, want 1", chunks[0].Meta.RowIndex)
	}
}

func TestMakeChunks_ShortRow(t *testing.T) {
	// Row has fewer columns than the schema — missing → empty → skipped.
	tbl := makeTable(
		[]string{"a", "b", "c"},
		[]string{"A", "B", "C"},
		[][]string{
			{"only_a"}, // cols B and C missing
		},
		"",
	)

	chunks := chunker.MakeChunks(tbl, "", "test-file-id")
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Text != "A: only_a" {
		t.Errorf("got %q, want %q", chunks[0].Text, "A: only_a")
	}
}

func TestMakeChunks_FallbackToNormalizedName(t *testing.T) {
	// OriginalName is empty → fall back to normalized Name.
	cols := []schema.ColumnDef{
		{Index: 0, Name: "col_1", OriginalName: "", Type: schema.TypeVarchar},
	}
	tbl := ingest.NormalizedTable{
		RawTable: ingest.RawTable{
			SourceFile: "f.csv",
			Rows:       [][]string{{"hello"}},
		},
		Columns:  cols,
		RowCount: 1,
	}

	chunks := chunker.MakeChunks(tbl, "", "test-file-id")
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk")
	}
	if chunks[0].Text != "col_1: hello" {
		t.Errorf("got %q, want %q", chunks[0].Text, "col_1: hello")
	}
}
