package schema

import (
	"fmt"
	"strings"
	"unicode"
)

// NormalizeHeaders converts raw header strings into deduplicated snake_case column names.
//
// Rules applied in order:
//  1. Trim whitespace; map non-alphanumeric runes to underscore; collapse runs of
//     underscores; strip leading/trailing underscores.
//  2. Empty result → "col_N" (1-based column index).
//  3. Duplicate normalized names → append "_2", "_3", … (first occurrence keeps base name).
func NormalizeHeaders(headers []string) []ColumnDef {
	cols := make([]ColumnDef, len(headers))
	seen := make(map[string]int) // base name → count of uses so far

	for i, h := range headers {
		norm := normalizeOne(h)
		if norm == "" {
			norm = fmt.Sprintf("col_%d", i+1)
		}

		base := norm
		seen[base]++
		if seen[base] > 1 {
			norm = fmt.Sprintf("%s_%d", base, seen[base])
		}

		cols[i] = ColumnDef{
			Index:        i,
			Name:         norm,
			OriginalName: h,
		}
	}
	return cols
}

func normalizeOne(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevUS := true // suppress leading underscores
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			prevUS = false
		} else {
			if !prevUS {
				b.WriteByte('_')
				prevUS = true
			}
		}
	}
	return strings.TrimRight(b.String(), "_")
}
