package duckdb_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/marcboeker/go-duckdb"

	"github.com/ETretyakov/rag-with-tables/internal/ingest/schema"
	duckstore "github.com/ETretyakov/rag-with-tables/internal/storage/duckdb"
)

func TestToParquet_BasicTable(t *testing.T) {
	cols := []schema.ColumnDef{
		{Index: 0, Name: "id", Type: schema.TypeInteger},
		{Index: 1, Name: "name", Type: schema.TypeVarchar},
		{Index: 2, Name: "score", Type: schema.TypeDouble},
	}
	rows := [][]string{
		{"1", "Alice", "95.5"},
		{"2", "Bob", "87.0"},
		{"3", "Charlie", "100.0"},
	}

	data, err := duckstore.ToParquet(context.Background(), cols, rows)
	if err != nil {
		t.Fatalf("ToParquet: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ToParquet returned empty bytes")
	}

	// Verify the Parquet file is readable and has the correct row count.
	tmp := filepath.Join(t.TempDir(), "out.parquet")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var count int
	row := db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM read_parquet(?)", tmp)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("read_parquet: %v", err)
	}
	if count != len(rows) {
		t.Errorf("row count: got %d, want %d", count, len(rows))
	}
}

func TestToParquet_EmptyRows(t *testing.T) {
	cols := []schema.ColumnDef{
		{Index: 0, Name: "id", Type: schema.TypeInteger},
		{Index: 1, Name: "label", Type: schema.TypeVarchar},
	}

	data, err := duckstore.ToParquet(context.Background(), cols, nil)
	if err != nil {
		t.Fatalf("ToParquet with empty rows: %v", err)
	}
	// Should produce a valid (empty) Parquet file.
	if len(data) == 0 {
		t.Fatal("expected non-empty parquet even for empty table")
	}
}

func TestToParquet_NullValues(t *testing.T) {
	cols := []schema.ColumnDef{
		{Index: 0, Name: "id", Type: schema.TypeInteger},
		{Index: 1, Name: "val", Type: schema.TypeDouble},
	}
	rows := [][]string{
		{"1", ""}, // empty → NULL for DOUBLE
		{"2", "3.14"},
		{"", "2.71"}, // empty → NULL for INTEGER
	}

	data, err := duckstore.ToParquet(context.Background(), cols, rows)
	if err != nil {
		t.Fatalf("ToParquet with nulls: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "nulls.parquet")
	os.WriteFile(tmp, data, 0o644)

	db, _ := sql.Open("duckdb", "")
	defer db.Close()

	var count int
	db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM read_parquet(?)", tmp).Scan(&count)
	if count != 3 {
		t.Errorf("row count: got %d, want 3", count)
	}
}

func TestToParquet_AllTypes(t *testing.T) {
	cols := []schema.ColumnDef{
		{Index: 0, Name: "b", Type: schema.TypeBoolean},
		{Index: 1, Name: "i", Type: schema.TypeInteger},
		{Index: 2, Name: "d", Type: schema.TypeDouble},
		{Index: 3, Name: "s", Type: schema.TypeVarchar},
		{Index: 4, Name: "dt", Type: schema.TypeDate},
		{Index: 5, Name: "ts", Type: schema.TypeTimestamp},
	}
	rows := [][]string{
		{"true", "42", "3.14", "hello", "2024-01-15", "2024-01-15 10:30:00"},
		{"false", "0", "-1.5", "world", "2023-12-31", "2023-12-31 23:59:59"},
	}

	data, err := duckstore.ToParquet(context.Background(), cols, rows)
	if err != nil {
		t.Fatalf("ToParquet all types: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty parquet")
	}
}
