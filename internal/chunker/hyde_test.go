package chunker_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ETretyakov/rag-with-tables/internal/chunker"
	"github.com/ETretyakov/rag-with-tables/internal/ingest"
	"github.com/ETretyakov/rag-with-tables/internal/ingest/schema"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
)

// mockLLM is a test double that returns a canned response.
type mockLLM struct {
	response string
	calls    atomic.Int64
}

func (m *mockLLM) Complete(_ context.Context, _ []provider.Message) (string, error) {
	m.calls.Add(1)
	return m.response, nil
}

func employeeTable() ingest.NormalizedTable {
	cols := []schema.ColumnDef{
		{Index: 0, Name: "name", OriginalName: "Name", Type: schema.TypeVarchar},
		{Index: 1, Name: "department", OriginalName: "Department", Type: schema.TypeVarchar},
		{Index: 2, Name: "salary", OriginalName: "Salary", Type: schema.TypeDouble},
	}
	return ingest.NormalizedTable{
		RawTable: ingest.RawTable{
			SourceFile: "employees.xlsx",
			SheetName:  "Q1",
			TableIndex: 0,
			Rows: [][]string{
				{"Alice", "Engineering", "90000"},
				{"Bob", "Sales", "65000"},
			},
		},
		Columns:  cols,
		S3Key:    "tables/emp.parquet",
		RowCount: 2,
	}
}

func TestGenerateHYDE_NilLLM(t *testing.T) {
	hyde, err := chunker.GenerateHYDE(context.Background(), employeeTable(), nil)
	if err != nil {
		t.Fatalf("nil LLM should not return error: %v", err)
	}
	if hyde != "" {
		t.Errorf("nil LLM should return empty HYDE, got %q", hyde)
	}
}

func TestGenerateHYDE_WithMockLLM(t *testing.T) {
	const fakeQuestions = "What is Alice's salary?\nWhich department has the highest salary?"
	llm := &mockLLM{response: fakeQuestions}

	hyde, err := chunker.GenerateHYDE(context.Background(), employeeTable(), llm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hyde != fakeQuestions {
		t.Errorf("got %q, want %q", hyde, fakeQuestions)
	}
	if llm.calls.Load() != 1 {
		t.Errorf("expected 1 LLM call, got %d", llm.calls.Load())
	}
}

func TestGenerateHYDEBatch_Parallel(t *testing.T) {
	tbl := employeeTable()
	tables := []ingest.NormalizedTable{tbl, tbl, tbl}

	llm := &mockLLM{response: "question"}

	hydes, err := chunker.GenerateHYDEBatch(context.Background(), tables, llm, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(hydes) != 3 {
		t.Fatalf("want 3 hydes, got %d", len(hydes))
	}
	for i, h := range hydes {
		if h != "question" {
			t.Errorf("hydes[%d] = %q, want %q", i, h, "question")
		}
	}
}

func TestChunkify_EndToEnd(t *testing.T) {
	tbl := employeeTable()
	llm := &mockLLM{response: "What is the salary of Alice?"}

	allChunks, err := chunker.Chunkify(context.Background(), []ingest.NormalizedTable{tbl}, llm, 1, "test-file-id")
	if err != nil {
		t.Fatal(err)
	}
	if len(allChunks) != 1 {
		t.Fatalf("want 1 table's chunks, got %d", len(allChunks))
	}

	chunks := allChunks[0]
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks (2 rows), got %d", len(chunks))
	}

	// Every chunk should carry the HYDE text.
	for i, ch := range chunks {
		if !strings.Contains(ch.Meta.HYDE, "salary") {
			t.Errorf("chunk[%d] HYDE missing: %q", i, ch.Meta.HYDE)
		}
		if ch.Meta.S3Key != "tables/emp.parquet" {
			t.Errorf("chunk[%d] S3Key: %q", i, ch.Meta.S3Key)
		}
	}

	// Spot-check chunk text.
	if !strings.Contains(chunks[0].Text, "Name: Alice") {
		t.Errorf("chunk[0].Text: %q", chunks[0].Text)
	}
}
