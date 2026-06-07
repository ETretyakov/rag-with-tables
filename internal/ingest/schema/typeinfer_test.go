package schema_test

import (
	"strings"
	"testing"

	"github.com/ETretyakov/rag-with-tables/internal/ingest/schema"
)

func TestInferColumnTypes(t *testing.T) {
	mkRows := func(vals ...string) [][]string {
		rows := make([][]string, len(vals))
		for i, v := range vals {
			rows[i] = []string{v}
		}
		return rows
	}

	col := func(t schema.ColumnType) schema.ColumnDef {
		return schema.ColumnDef{Index: 0, Name: "v", Type: t}
	}

	tests := []struct {
		name   string
		values []string
		want   schema.ColumnType
	}{
		// BOOLEAN
		{"all true/false", []string{"true", "false", "true", "FALSE", "TRUE"}, schema.TypeBoolean},
		{"yes/no variants", []string{"yes", "no", "yes", "YES", "NO"}, schema.TypeBoolean},
		{"t/f variants", []string{"t", "f", "T", "F", "t"}, schema.TypeBoolean},

		// INTEGER
		{"integers", []string{"1", "2", "3", "-10", "0"}, schema.TypeInteger},
		{"integers with empties (< 5% bad)", func() []string {
			s := make([]string, 100)
			for i := range s {
				s[i] = "42"
			}
			s[99] = "" // empty, not bad — gets ignored as non-empty sample
			return s
		}(), schema.TypeInteger},

		// DOUBLE
		{"floats", []string{"1.5", "2.3", "-0.1", "100.0", "3.14"}, schema.TypeDouble},
		{"mixed int and float", []string{"1", "2.5", "3", "4.0", "5"}, schema.TypeDouble},
		{"currency dollar", []string{"$1,234.56", "$500.00", "$12,000.00", "$750.50"}, schema.TypeDouble},
		{"currency euro", []string{"€1,234.56", "€500.00", "€12,000.00"}, schema.TypeDouble},
		{"percent", []string{"50%", "75.5%", "100%", "12.3%"}, schema.TypeDouble},

		// DATE
		{"iso dates", []string{"2024-01-15", "2023-12-31", "2025-06-01"}, schema.TypeDate},
		{"dot dates", []string{"15.01.2024", "31.12.2023", "01.06.2025"}, schema.TypeDate},

		// TIMESTAMP
		{"iso timestamps", []string{
			"2024-01-15 10:30:00",
			"2023-12-31 23:59:59",
			"2025-06-01 00:00:01",
		}, schema.TypeTimestamp},
		{"rfc3339 timestamps", []string{
			"2024-01-15T10:30:00Z",
			"2023-12-31T23:59:59Z",
		}, schema.TypeTimestamp},

		// VARCHAR fallback
		{"mixed types → varchar", []string{"1", "two", "3", "four"}, schema.TypeVarchar},
		{"all empty → varchar", []string{"", "", ""}, schema.TypeVarchar},
		{"text → varchar", []string{"hello", "world", "foo"}, schema.TypeVarchar},

		// Threshold: 95% rule — need < 95% to fall back to VARCHAR
		{"90% integers + 10% text → varchar", func() []string {
			s := make([]string, 20)
			for i := range s {
				s[i] = "42"
			}
			// 2 bad values out of 20 = 90% parseable → below 95% threshold → VARCHAR
			s[0] = "not_a_number"
			s[1] = "also_bad"
			return s
		}(), schema.TypeVarchar},
		{"96% integers + 4% empty → integer", func() []string {
			s := make([]string, 25)
			for i := range s {
				s[i] = "42"
			}
			s[0] = "" // empty — excluded from non-empty sample
			return s
		}(), schema.TypeInteger},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseCols := []schema.ColumnDef{{Index: 0, Name: "v"}}
			rows := mkRows(tt.values...)
			got := schema.InferColumnTypes(baseCols, rows)
			if got[0].Type != tt.want {
				t.Errorf("got %s, want %s (sample: %s)",
					got[0].Type, tt.want,
					strings.Join(tt.values[:min(3, len(tt.values))], ", "))
			}
			_ = col(tt.want)
		})
	}
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		colType schema.ColumnType
		input   string
		wantNil bool
		wantStr string // non-empty → check string repr
	}{
		{schema.TypeVarchar, "hello", false, "hello"},
		{schema.TypeVarchar, "", true, ""},
		{schema.TypeInteger, "42", false, ""},
		{schema.TypeInteger, "", true, ""},
		{schema.TypeInteger, "abc", true, ""},
		{schema.TypeDouble, "3.14", false, ""},
		{schema.TypeBoolean, "true", false, ""},
		{schema.TypeBoolean, "false", false, ""},
		{schema.TypeDate, "2024-01-15", false, ""},
		{schema.TypeTimestamp, "2024-01-15T10:30:00Z", false, ""},
	}

	for _, tt := range tests {
		col := schema.ColumnDef{Type: tt.colType}
		got := col.ParseValue(tt.input)

		if tt.wantNil && got != nil {
			t.Errorf("type=%s input=%q: want nil, got %v", tt.colType, tt.input, got)
		}
		if !tt.wantNil && got == nil {
			t.Errorf("type=%s input=%q: want non-nil, got nil", tt.colType, tt.input)
		}
		if tt.wantStr != "" {
			if s, ok := got.(string); !ok || s != tt.wantStr {
				t.Errorf("type=%s input=%q: want %q, got %v", tt.colType, tt.input, tt.wantStr, got)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
