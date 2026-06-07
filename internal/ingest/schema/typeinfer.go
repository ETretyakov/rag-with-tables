package schema

import (
	"strconv"
	"strings"
	"time"
)

const inferThreshold = 0.95 // fraction of non-empty values that must match a type

// Shared time-format lists used by both InferColumnTypes and ColumnDef.ParseValue.
var DateFormats = []string{
	"2006-01-02",
	"02.01.2006",
	"01/02/2006",
	"2006/01/02",
	"02 Jan 2006",
	"Jan 02, 2006",
	"02-Jan-2006",
}

var TimestampFormats = []string{
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999",
	"02.01.2006 15:04:05",
	"01/02/2006 15:04:05",
}

var boolTrueSet = map[string]bool{
	"true": true, "yes": true, "t": true, "y": true,
}
var boolFalseSet = map[string]bool{
	"false": true, "no": true, "f": true, "n": true,
}

// InferColumnTypes sets the Type field on each ColumnDef by sampling data rows.
// Columns are tested in specificity order: BOOLEAN → INTEGER → DOUBLE → DATE → TIMESTAMP → VARCHAR.
// A type is chosen when ≥95 % of non-empty values in the column successfully parse as that type.
func InferColumnTypes(cols []ColumnDef, rows [][]string) []ColumnDef {
	out := make([]ColumnDef, len(cols))
	copy(out, cols)
	for i := range out {
		out[i].Type = inferOne(columnValues(rows, i))
	}
	return out
}

func columnValues(rows [][]string, col int) []string {
	vals := make([]string, 0, len(rows))
	for _, row := range rows {
		if col < len(row) {
			vals = append(vals, strings.TrimSpace(row[col]))
		}
	}
	return vals
}

func inferOne(values []string) ColumnType {
	nonEmpty := nonEmptyOnly(values)
	if len(nonEmpty) == 0 {
		return TypeVarchar
	}

	candidates := []struct {
		t  ColumnType
		fn func(string) bool
	}{
		{TypeBoolean, isBoolean},
		{TypeInteger, isInteger},
		{TypeDouble, isDouble},
		{TypeDate, isDate},
		{TypeTimestamp, isTimestamp},
	}

	for _, c := range candidates {
		if matchRate(nonEmpty, c.fn) >= inferThreshold {
			return c.t
		}
	}
	return TypeVarchar
}

func nonEmptyOnly(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func matchRate(vals []string, fn func(string) bool) float64 {
	ok := 0
	for _, v := range vals {
		if fn(v) {
			ok++
		}
	}
	return float64(ok) / float64(len(vals))
}

func isBoolean(s string) bool {
	l := strings.ToLower(s)
	return boolTrueSet[l] || boolFalseSet[l]
}

func isInteger(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func isDouble(s string) bool {
	_, err := strconv.ParseFloat(normNumeric(s), 64)
	return err == nil
}

// normNumeric strips common currency symbols ($€£¥₹), trailing percent signs,
// and comma thousands-separators so that values like "$1,234.56" parse as float.
// Only removes commas when a decimal point is also present (e.g. "1,234.56" →
// "1234.56"); a bare "1,234" without a dot is left unchanged to avoid
// misinterpreting European decimal commas.
func normNumeric(s string) string {
	s = strings.TrimLeft(s, " $€£¥₹")
	s = strings.TrimRight(s, "% ")
	s = strings.TrimSpace(s)
	if strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", "")
	}
	return s
}

func isDate(s string) bool {
	for _, f := range DateFormats {
		if _, err := time.Parse(f, s); err == nil {
			return true
		}
	}
	return false
}

func isTimestamp(s string) bool {
	for _, f := range TimestampFormats {
		if _, err := time.Parse(f, s); err == nil {
			return true
		}
	}
	return false
}
