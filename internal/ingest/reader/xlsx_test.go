package reader_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/ETretyakov/rag-with-tables/internal/ingest/reader"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func cell(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

func newFile(sheetName string) *excelize.File {
	f := excelize.NewFile()
	f.SetSheetName("Sheet1", sheetName)
	return f
}

// toReader serialises an excelize.File to an in-memory io.Reader.
func toReader(t *testing.T, f *excelize.File) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx to buffer: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func extract(t *testing.T, name string, rd *bytes.Reader) []interface{ dummy() } {
	t.Helper()
	return nil
}

// ── basic tests ───────────────────────────────────────────────────────────────

func TestXLSXReader_SingleTable(t *testing.T) {
	f := newFile("Sheet1")
	f.SetCellValue("Sheet1", cell(1, 1), "Name")
	f.SetCellValue("Sheet1", cell(2, 1), "Age")
	f.SetCellValue("Sheet1", cell(1, 2), "Alice")
	f.SetCellValue("Sheet1", cell(2, 2), "30")
	f.SetCellValue("Sheet1", cell(1, 3), "Bob")
	f.SetCellValue("Sheet1", cell(2, 3), "25")

	r, err := reader.New("test.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	tables, err := r.Extract(context.Background(), toReader(t, f), "test.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(tables))
	}
	tbl := tables[0]
	if len(tbl.Headers) != 2 || tbl.Headers[0] != "Name" || tbl.Headers[1] != "Age" {
		t.Errorf("headers: %v", tbl.Headers)
	}
	if len(tbl.Rows) != 2 {
		t.Errorf("rows: want 2, got %d", len(tbl.Rows))
	}
}

func TestXLSXReader_TwoTablesOnSheet(t *testing.T) {
	f := newFile("Data")
	f.SetCellValue("Data", cell(1, 1), "A")
	f.SetCellValue("Data", cell(2, 1), "B")
	f.SetCellValue("Data", cell(1, 2), "1")
	f.SetCellValue("Data", cell(2, 2), "2")
	f.SetCellValue("Data", cell(4, 1), "X")
	f.SetCellValue("Data", cell(5, 1), "Y")
	f.SetCellValue("Data", cell(4, 2), "3")
	f.SetCellValue("Data", cell(5, 2), "4")

	r, _ := reader.New("data.xlsx")
	tables, err := r.Extract(context.Background(), toReader(t, f), "data.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 {
		t.Fatalf("want 2 tables, got %d", len(tables))
	}
}

func TestXLSXReader_MultipleSheets(t *testing.T) {
	f := newFile("January")
	f.NewSheet("February")

	for sheet, hdr := range map[string]string{"January": "Sales", "February": "Revenue"} {
		f.SetCellValue(sheet, cell(1, 1), "Month")
		f.SetCellValue(sheet, cell(2, 1), hdr)
		f.SetCellValue(sheet, cell(1, 2), "Jan")
		f.SetCellValue(sheet, cell(2, 2), "100")
	}

	r, _ := reader.New("multi.xlsx")
	tables, err := r.Extract(context.Background(), toReader(t, f), "multi.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 2 {
		t.Fatalf("want 2 tables (one per sheet), got %d", len(tables))
	}
}

// ── merged title cell ─────────────────────────────────────────────────────────

func TestXLSXReader_MergedTitleCell(t *testing.T) {
	f := newFile("Data")
	f.SetCellValue("Data", cell(1, 1), "Sales Report")
	f.MergeCell("Data", cell(1, 1), cell(4, 1))
	f.SetCellValue("Data", cell(1, 2), "Month")
	f.SetCellValue("Data", cell(2, 2), "Category")
	f.SetCellValue("Data", cell(3, 2), "Sales")
	f.SetCellValue("Data", cell(4, 2), "Profit")
	f.SetCellValue("Data", cell(1, 3), "Jan")
	f.SetCellValue("Data", cell(2, 3), "A")
	f.SetCellValue("Data", cell(3, 3), "100")
	f.SetCellValue("Data", cell(4, 3), "50")

	r, _ := reader.New("merged.xlsx")
	tables, err := r.Extract(context.Background(), toReader(t, f), "merged.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(tables))
	}
	tbl := tables[0]
	wantHeaders := []string{"Month", "Category", "Sales", "Profit"}
	if len(tbl.Headers) != len(wantHeaders) {
		t.Fatalf("headers: got %v, want %v", tbl.Headers, wantHeaders)
	}
	for i, h := range tbl.Headers {
		if h != wantHeaders[i] {
			t.Errorf("headers[%d]: got %q, want %q", i, h, wantHeaders[i])
		}
	}
}

func TestXLSXReader_MergedTitleThenMultipleDataRows(t *testing.T) {
	f := newFile("Sheet1")
	f.SetCellValue("Sheet1", cell(1, 1), "Q1 Report")
	f.MergeCell("Sheet1", cell(1, 1), cell(3, 1))
	for col, h := range []string{"Product", "Units", "Revenue"} {
		f.SetCellValue("Sheet1", cell(col+1, 2), h)
	}
	data := [][]string{{"A", "10", "500"}, {"B", "20", "800"}, {"C", "5", "200"}}
	for row, rec := range data {
		for col, v := range rec {
			f.SetCellValue("Sheet1", cell(col+1, row+3), v)
		}
	}

	r, _ := reader.New("q1.xlsx")
	tables, err := r.Extract(context.Background(), toReader(t, f), "q1.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(tables))
	}
	tbl := tables[0]
	if tbl.Headers[0] != "Product" {
		t.Errorf("first header: %q", tbl.Headers[0])
	}
	if len(tbl.Rows) != 3 {
		t.Errorf("rows: want 3, got %d", len(tbl.Rows))
	}
}

func TestXLSXReader_TableNotAtTopLeft(t *testing.T) {
	f := newFile("Sheet1")
	f.SetCellValue("Sheet1", cell(3, 4), "ID")
	f.SetCellValue("Sheet1", cell(4, 4), "Value")
	f.SetCellValue("Sheet1", cell(3, 5), "1")
	f.SetCellValue("Sheet1", cell(4, 5), "100")
	f.SetCellValue("Sheet1", cell(3, 6), "2")
	f.SetCellValue("Sheet1", cell(4, 6), "200")

	r, _ := reader.New("offset.xlsx")
	tables, err := r.Extract(context.Background(), toReader(t, f), "offset.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(tables))
	}
	tbl := tables[0]
	if len(tbl.Headers) != 2 || tbl.Headers[0] != "ID" {
		t.Errorf("headers: %v", tbl.Headers)
	}
}

func TestXLSXReader_MergedTitleWithTableOffset(t *testing.T) {
	f := newFile("Report")
	f.SetCellValue("Report", cell(3, 2), "Department Report")
	f.MergeCell("Report", cell(3, 2), cell(5, 2))
	f.SetCellValue("Report", cell(3, 3), "Name")
	f.SetCellValue("Report", cell(4, 3), "Role")
	f.SetCellValue("Report", cell(5, 3), "Salary")
	f.SetCellValue("Report", cell(3, 4), "Alice")
	f.SetCellValue("Report", cell(4, 4), "Engineer")
	f.SetCellValue("Report", cell(5, 4), "90000")

	r, _ := reader.New("report.xlsx")
	tables, err := r.Extract(context.Background(), toReader(t, f), "report.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(tables))
	}
	if tables[0].Headers[0] != "Name" {
		t.Errorf("first header: %q", tables[0].Headers[0])
	}
}

func TestXLSXReader_PhantomHeaderColumns(t *testing.T) {
	f := newFile("Sheet1")
	for col, h := range []string{"Name", "Score", "H3_nodata", "H4_nodata", "H5_nodata"} {
		f.SetCellValue("Sheet1", cell(col+1, 1), h)
	}
	f.SetCellValue("Sheet1", cell(1, 2), "Alice")
	f.SetCellValue("Sheet1", cell(2, 2), "95")
	f.SetCellValue("Sheet1", cell(1, 3), "Bob")
	f.SetCellValue("Sheet1", cell(2, 3), "87")

	r, _ := reader.New("phantom.xlsx")
	tables, err := r.Extract(context.Background(), toReader(t, f), "phantom.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(tables))
	}
	if len(tables[0].Headers) != 2 {
		t.Errorf("headers: want 2 (phantom excluded), got %d: %v",
			len(tables[0].Headers), tables[0].Headers)
	}
}

// ── CSV ───────────────────────────────────────────────────────────────────────

func TestCSVReader(t *testing.T) {
	content := "id,name,value\n1,foo,10\n2,bar,20\n"
	r, err := reader.New("data.csv")
	if err != nil {
		t.Fatal(err)
	}
	tables, err := r.Extract(context.Background(), strings.NewReader(content), "data.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("want 1 table, got %d", len(tables))
	}
	if len(tables[0].Headers) != 3 {
		t.Errorf("headers: want 3, got %d", len(tables[0].Headers))
	}
	if len(tables[0].Rows) != 2 {
		t.Errorf("rows: want 2, got %d", len(tables[0].Rows))
	}
}

func TestReaderFactory_UnsupportedFormat(t *testing.T) {
	_, err := reader.New("file.json")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

// Verify that the reader works with a real file on disk via os.Open.
func TestCSVReader_FromDiskFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "data.csv")
	if err := os.WriteFile(tmp, []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r, _ := reader.New(tmp)
	tables, err := r.Extract(context.Background(), f, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || len(tables[0].Headers) != 2 {
		t.Errorf("unexpected: %v", tables)
	}
}
