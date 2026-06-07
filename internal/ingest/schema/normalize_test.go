package schema_test

import (
	"testing"

	"github.com/ETretyakov/rag-with-tables/internal/ingest/schema"
)

func TestNormalizeHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		want    []string // normalized names
	}{
		{
			name:    "basic snake_case",
			headers: []string{"First Name", "Last Name", "Age"},
			want:    []string{"first_name", "last_name", "age"},
		},
		{
			name:    "trim and lowercase",
			headers: []string{"  TOTAL  ", "  Revenue USD  "},
			want:    []string{"total", "revenue_usd"},
		},
		{
			name:    "special characters become underscores",
			headers: []string{"Price ($)", "Rate %", "A/B test"},
			want:    []string{"price", "rate", "a_b_test"},
		},
		{
			name:    "empty header gets col_N",
			headers: []string{"ID", "", "Value"},
			want:    []string{"id", "col_2", "value"},
		},
		{
			name:    "all empty headers",
			headers: []string{"", "", ""},
			want:    []string{"col_1", "col_2", "col_3"},
		},
		{
			name:    "duplicate names deduplicated",
			headers: []string{"Name", "Name", "Name"},
			want:    []string{"name", "name_2", "name_3"},
		},
		{
			name:    "duplicate after normalization",
			headers: []string{"First Name", "First-Name", "First  Name"},
			want:    []string{"first_name", "first_name_2", "first_name_3"},
		},
		{
			name:    "leading/trailing special chars stripped",
			headers: []string{"_hidden_", "---value---"},
			want:    []string{"hidden", "value"},
		},
		{
			name:    "digits preserved",
			headers: []string{"col1", "value2", "Q4 Revenue"},
			want:    []string{"col1", "value2", "q4_revenue"},
		},
		{
			// unicode.IsLetter returns true for Cyrillic — letters are kept (lowercased).
			// DuckDB handles non-ASCII identifiers when quoted.
			name:    "unicode (Cyrillic) letters lowercased and kept",
			headers: []string{"Количество"},
			want:    []string{"количество"},
		},
		{
			name:    "single-char headers",
			headers: []string{"A", "B", "C"},
			want:    []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schema.NormalizeHeaders(tt.headers)
			if len(got) != len(tt.want) {
				t.Fatalf("len: got %d, want %d", len(got), len(tt.want))
			}
			for i, col := range got {
				if col.Name != tt.want[i] {
					t.Errorf("[%d] %q → got %q, want %q", i, tt.headers[i], col.Name, tt.want[i])
				}
				if col.OriginalName != tt.headers[i] {
					t.Errorf("[%d] OriginalName: got %q, want %q", i, col.OriginalName, tt.headers[i])
				}
				if col.Index != i {
					t.Errorf("[%d] Index: got %d, want %d", i, col.Index, i)
				}
			}
		})
	}
}
