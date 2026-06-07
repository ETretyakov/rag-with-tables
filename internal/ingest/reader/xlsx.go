package reader

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/ETretyakov/rag-with-tables/internal/ingest"
	"github.com/ETretyakov/rag-with-tables/internal/ingest/islands"
)

type xlsxReader struct{}

func (r *xlsxReader) Extract(_ context.Context, rd io.Reader, name string) ([]ingest.RawTable, error) {
	// excelize.OpenReader reads directly from the io.Reader — no temp file needed.
	f, err := excelize.OpenReader(rd)
	if err != nil {
		return nil, fmt.Errorf("open xlsx %s: %w", name, err)
	}
	defer f.Close()

	sourceName := filepath.Base(name)
	var tables []ingest.RawTable

	for _, sheetName := range f.GetSheetList() {
		grid, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("get rows sheet %q: %w", sheetName, err)
		}

		merges, _ := f.GetMergeCells(sheetName)

		for i, b := range islands.Detect(grid) {
			b = skipTitleRows(b, grid, merges)
			if b.MinRow > b.MaxRow {
				continue
			}
			tables = append(tables, extractFromGrid(grid, sourceName, sheetName, i, b))
		}
	}

	return tables, nil
}

// skipTitleRows advances MinRow past any leading title rows.
func skipTitleRows(b islands.Bounds, grid [][]string, merges []excelize.MergeCell) islands.Bounds {
	for b.MinRow < b.MaxRow && isTitleRow(b.MinRow, b.MinCol, b.MaxCol, grid, merges) {
		b.MinRow++
	}
	return b
}

func isTitleRow(row, minCol, maxCol int, grid [][]string, merges []excelize.MergeCell) bool {
	for _, m := range merges {
		sc, sr, err1 := excelize.CellNameToCoordinates(m.GetStartAxis())
		ec, er, err2 := excelize.CellNameToCoordinates(m.GetEndAxis())
		if err1 != nil || err2 != nil {
			continue
		}
		sr--
		sc--
		er--
		ec--
		if sr == row && er == row {
			overlapStart := max(sc, minCol)
			overlapEnd := min(ec, maxCol)
			if overlapEnd-overlapStart >= 1 {
				return true
			}
		}
	}

	if row+1 >= len(grid) || maxCol <= minCol {
		return false
	}
	nonEmpty := func(r int) int {
		n := 0
		for c := minCol; c <= maxCol; c++ {
			if c < len(grid[r]) && strings.TrimSpace(grid[r][c]) != "" {
				n++
			}
		}
		return n
	}
	return nonEmpty(row) == 1 && nonEmpty(row+1) > 1
}

func extractFromGrid(grid [][]string, sourceFile, sheetName string, idx int, b islands.Bounds) ingest.RawTable {
	width := b.MaxCol - b.MinCol + 1

	cell := func(r, c int) string {
		if r >= len(grid) || c >= len(grid[r]) {
			return ""
		}
		return grid[r][c]
	}

	headers := make([]string, width)
	for c := b.MinCol; c <= b.MaxCol; c++ {
		headers[c-b.MinCol] = cell(b.MinRow, c)
	}

	var rows [][]string
	for r := b.MinRow + 1; r <= b.MaxRow; r++ {
		row := make([]string, width)
		for c := b.MinCol; c <= b.MaxCol; c++ {
			row[c-b.MinCol] = cell(r, c)
		}
		rows = append(rows, row)
	}

	return ingest.RawTable{
		SourceFile: sourceFile,
		SheetName:  sheetName,
		TableIndex: idx,
		Headers:    headers,
		Rows:       rows,
		Bounds: ingest.TableBounds{
			SheetName: sheetName,
			StartRow:  b.MinRow,
			StartCol:  b.MinCol,
			EndRow:    b.MaxRow,
			EndCol:    b.MaxCol,
		},
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
