// Package islands implements table region detection in sparse cell grids.
//
// The algorithm works in two steps:
//
//  1. Group consecutive non-empty rows into "row bands" (a fully-empty row is a
//     separator between tables).
//
//  2. For each band, determine the column extent using DATA rows (rows after the
//     first row of the band). This prevents auto-generated / phantom header
//     columns — columns that have a header but zero data cells — from bloating
//     the island's width (e.g. Excel can auto-generate 16 384 column headers).
//     Columns that are interior to the data range but happen to be sparse (a
//     header exists, data cells are missing in some rows) are still included.
//     Only columns that lie entirely OUTSIDE the data range and carry no data
//     are dropped.
//
//     Special case — header-only bands (single row): the original "any
//     non-empty cell" rule is used because there are no data rows to infer the
//     range from.
package islands

import "strings"

// Bounds is the bounding rectangle of a detected table island (0-based row/col indices).
type Bounds struct {
	MinRow, MaxRow int
	MinCol, MaxCol int
}

// Detect finds table islands in a ragged cell grid.
// grid[r] may be shorter than the maximum column count — missing cells are treated as empty.
func Detect(grid [][]string) []Bounds {
	if len(grid) == 0 {
		return nil
	}

	maxCols := 0
	for _, row := range grid {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	if maxCols == 0 {
		return nil
	}

	cellEmpty := func(r, c int) bool {
		if r >= len(grid) || c >= len(grid[r]) {
			return true
		}
		return strings.TrimSpace(grid[r][c]) == ""
	}

	rowEmpty := func(r int) bool {
		for c := 0; c < maxCols; c++ {
			if !cellEmpty(r, c) {
				return false
			}
		}
		return true
	}

	// Step 1 — collect row bands.
	type band struct{ start, end int }
	var bands []band
	bandStart, inBand := 0, false
	for r := range grid {
		if !rowEmpty(r) {
			if !inBand {
				bandStart, inBand = r, true
			}
		} else if inBand {
			bands = append(bands, band{bandStart, r - 1})
			inBand = false
		}
	}
	if inBand {
		bands = append(bands, band{bandStart, len(grid) - 1})
	}

	// Step 2 — for each band find column groups.
	var result []Bounds
	for _, b := range bands {
		if b.start == b.end {
			// Header-only band: consecutive non-empty columns in the single row.
			result = append(result, colGroupsFromRow(b.start, b.start, maxCols, cellEmpty)...)
		} else {
			result = append(result, colGroupsFromData(b.start, b.end, maxCols, cellEmpty)...)
		}
	}
	return result
}

// colGroupsFromRow finds column groups using a single row — used for header-only bands.
func colGroupsFromRow(rowMin, rowMax, maxCols int, cellEmpty func(r, c int) bool) []Bounds {
	var result []Bounds
	colStart, inGroup := 0, false
	for c := 0; c < maxCols; c++ {
		a := !cellEmpty(rowMin, c)
		switch {
		case a && !inGroup:
			colStart, inGroup = c, true
		case !a && inGroup:
			result = append(result, Bounds{rowMin, rowMax, colStart, c - 1})
			inGroup = false
		}
	}
	if inGroup {
		result = append(result, Bounds{rowMin, rowMax, colStart, maxCols - 1})
	}
	return result
}

// colGroupsFromData finds column groups for a multi-row band using the data-extent rule:
//
//   - Scan DATA rows (headerRow+1 … bandEnd) to find leftData / rightData — the
//     leftmost and rightmost columns that actually contain cell values.
//   - Only columns in [leftData, rightData] are considered; columns outside this
//     range are excluded even if they carry a header (phantom columns).
//   - Within [leftData, rightData], a column is active if it has ANY non-empty
//     cell in the FULL band (header included), so sparse interior columns
//     that have a header but no data for some rows are still part of the table.
//   - Remaining empty columns inside the range serve as separators (two tables
//     with no content between them stay separate).
func colGroupsFromData(headerRow, bandEnd, maxCols int, cellEmpty func(r, c int) bool) []Bounds {
	// Find leftmost and rightmost column with data in non-header rows.
	leftData, rightData := maxCols, -1
	for c := 0; c < maxCols; c++ {
		for r := headerRow + 1; r <= bandEnd; r++ {
			if !cellEmpty(r, c) {
				if c < leftData {
					leftData = c
				}
				if c > rightData {
					rightData = c
				}
				break
			}
		}
	}

	// No data rows have any content — should not happen in a valid multi-row band,
	// but fall back to the header-row approach to stay correct.
	if leftData > rightData {
		return colGroupsFromRow(headerRow, bandEnd, maxCols, cellEmpty)
	}

	// Build active flags only within the data range.
	active := make([]bool, maxCols)
	for c := leftData; c <= rightData; c++ {
		for r := headerRow; r <= bandEnd; r++ {
			if !cellEmpty(r, c) {
				active[c] = true
				break
			}
		}
	}

	// Find consecutive active column groups within [leftData, rightData].
	var result []Bounds
	colStart, inGroup := 0, false
	for c := leftData; c <= rightData; c++ {
		switch {
		case active[c] && !inGroup:
			colStart, inGroup = c, true
		case !active[c] && inGroup:
			result = append(result, Bounds{headerRow, bandEnd, colStart, c - 1})
			inGroup = false
		}
	}
	if inGroup {
		result = append(result, Bounds{headerRow, bandEnd, colStart, rightData})
	}
	return result
}
