package islands_test

import (
	"testing"

	"github.com/ETretyakov/rag-with-tables/internal/ingest/islands"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		grid [][]string
		want []islands.Bounds
	}{
		// ── Trivial / empty ────────────────────────────────────────────────────

		{
			name: "nil grid",
			grid: nil,
			want: nil,
		},
		{
			name: "all empty rows",
			grid: [][]string{{}, {""}, {"", ""}},
			want: nil,
		},
		{
			name: "single cell",
			grid: [][]string{{"A"}},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 0, MinCol: 0, MaxCol: 0}},
		},

		// ── Basic tables ───────────────────────────────────────────────────────

		{
			name: "one simple table",
			grid: [][]string{
				{"Name", "Age"},
				{"Alice", "30"},
				{"Bob", "25"},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 2, MinCol: 0, MaxCol: 1}},
		},
		{
			name: "single-row header-only table",
			grid: [][]string{
				{"ID", "Name", "Value"},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 0, MinCol: 0, MaxCol: 2}},
		},

		// ── Row-band separation ────────────────────────────────────────────────

		{
			name: "two tables separated by one empty row",
			grid: [][]string{
				{"A", "B"},
				{"1", "2"},
				{},
				{"X", "Y"},
				{"3", "4"},
			},
			want: []islands.Bounds{
				{MinRow: 0, MaxRow: 1, MinCol: 0, MaxCol: 1},
				{MinRow: 3, MaxRow: 4, MinCol: 0, MaxCol: 1},
			},
		},
		{
			name: "two tables separated by multiple empty rows",
			grid: [][]string{
				{"H1", "H2"},
				{"v1", "v2"},
				{}, {}, {},
				{"H3", "H4"},
				{"v3", "v4"},
			},
			want: []islands.Bounds{
				{MinRow: 0, MaxRow: 1, MinCol: 0, MaxCol: 1},
				{MinRow: 5, MaxRow: 6, MinCol: 0, MaxCol: 1},
			},
		},

		// ── Column-group separation ────────────────────────────────────────────

		{
			name: "two tables side by side (one empty column separator)",
			grid: [][]string{
				{"A", "B", "", "D", "E"},
				{"1", "2", "", "3", "4"},
			},
			want: []islands.Bounds{
				{MinRow: 0, MaxRow: 1, MinCol: 0, MaxCol: 1},
				{MinRow: 0, MaxRow: 1, MinCol: 3, MaxCol: 4},
			},
		},

		// ── Offset / borders ───────────────────────────────────────────────────

		{
			// Table is buried several rows and columns in from the sheet origin.
			name: "table starts far from origin (row and col offset)",
			grid: [][]string{
				{}, {}, {},
				{"", "", "", "", ""},
				{"", "", "H1", "H2", "H3"},
				{"", "", "v1", "v2", "v3"},
				{"", "", "v4", "v5", "v6"},
				{}, {},
			},
			want: []islands.Bounds{{MinRow: 4, MaxRow: 6, MinCol: 2, MaxCol: 4}},
		},
		{
			// Leading empty columns; excelize trims trailing cells so only leading
			// empties appear in the row slices.
			name: "leading empty columns (xlsx trailing trimmed)",
			grid: [][]string{
				{"", "", "H1", "H2"},
				{"", "", "v1", "v2"},
				{"", "", "v3", "v4"},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 2, MinCol: 2, MaxCol: 3}},
		},
		{
			// Both leading and trailing empty columns explicitly present
			// (e.g. CSV or xlsx reader that pads rows to equal length).
			name: "leading and trailing empty columns (padded rows)",
			grid: [][]string{
				{"", "H1", "H2", ""},
				{"", "v1", "v2", ""},
				{"", "v3", "v4", ""},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 2, MinCol: 1, MaxCol: 2}},
		},

		// ── Phantom / auto-generated headers (KEY CASES) ──────────────────────

		{
			// Excel auto-generates N column headers but actual data is only in a
			// small subset. Phantom columns (header present, zero data cells) must
			// be excluded from the island.
			name: "phantom header columns after data range are excluded",
			grid: [][]string{
				{"H1", "H2", "H3_phantom", "H4_phantom"},
				{"v1", "v2"}, // trailing cells trimmed — col 2, 3 have no data
				{"v3", "v4"},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 2, MinCol: 0, MaxCol: 1}},
		},
		{
			// Phantom columns combined with leading empty column (realistic xlsx).
			name: "leading empty col + phantom trailing headers",
			grid: [][]string{
				{"", "H1", "H2", "H3_phantom", "H4_phantom"},
				{"", "v1", "v2"},
				{"", "v3", "v4"},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 2, MinCol: 1, MaxCol: 2}},
		},
		{
			// Phantom leading column: column has a header but no data and sits to
			// the LEFT of the actual data range.
			name: "phantom header column before data range is excluded",
			grid: [][]string{
				{"H_phantom", "H1", "H2"},
				{"", "v1", "v2"},
				{"", "v3", "v4"},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 2, MinCol: 1, MaxCol: 2}},
		},
		{
			// Interior sparse column: header exists, some data rows are empty for
			// it, but it sits BETWEEN two columns that do have data. Must be kept.
			name: "interior sparse column (header + some data) stays in island",
			grid: [][]string{
				{"H1", "H2", "H3"},
				{"v1", "", "v3"}, // col 1 empty in row 1
				{"v4", "v5", ""}, // col 2 empty in row 2
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 2, MinCol: 0, MaxCol: 2}},
		},

		// ── Sparse data rows ───────────────────────────────────────────────────

		{
			// Interior cells empty — the whole region is still one island.
			name: "sparse data rows form single island",
			grid: [][]string{
				{"Name", "Age", "City"},
				{"Alice", "30", ""}, // trailing empty trimmed by xlsx readers
				{"", "", "Moscow"},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 2, MinCol: 0, MaxCol: 2}},
		},
		{
			// Rows have different lengths because excelize trims trailing empties.
			// The island must still span all columns that had data somewhere.
			name: "variable-length rows (sparse trailing cells)",
			grid: [][]string{
				{"H1", "H2", "H3"},
				{"v1"},       // cols 1-2 trimmed
				{"v4", "v5"}, // col 2 trimmed
				{"v7", "v8", "v9"},
			},
			want: []islands.Bounds{{MinRow: 0, MaxRow: 3, MinCol: 0, MaxCol: 2}},
		},

		// ── Complex / combined ─────────────────────────────────────────────────

		{
			name: "multiple tables on one sheet",
			grid: [][]string{
				{"A", "B", "", "X", "Y"},
				{"1", "2", "", "5", "6"},
				{},
				{"", "", "", "P", "Q"},
				{"", "", "", "7", "8"},
			},
			want: []islands.Bounds{
				{MinRow: 0, MaxRow: 1, MinCol: 0, MaxCol: 1},
				{MinRow: 0, MaxRow: 1, MinCol: 3, MaxCol: 4},
				{MinRow: 3, MaxRow: 4, MinCol: 3, MaxCol: 4},
			},
		},
		{
			// Three tables: two side-by-side (with an offset from origin),
			// one below separated by empty rows.
			name: "three tables: two side-by-side with offset, one below",
			grid: [][]string{
				{},
				{"", "A", "B", "", "D", "E"},
				{"", "1", "2", "", "3", "4"},
				{}, {},
				{"X", "Y", "Z"},
				{"5", "6", "7"},
			},
			want: []islands.Bounds{
				{MinRow: 1, MaxRow: 2, MinCol: 1, MaxCol: 2},
				{MinRow: 1, MaxRow: 2, MinCol: 4, MaxCol: 5},
				{MinRow: 5, MaxRow: 6, MinCol: 0, MaxCol: 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := islands.Detect(tt.grid)
			if len(got) != len(tt.want) {
				t.Fatalf("island count: got %d, want %d\n  got:  %v\n  want: %v",
					len(got), len(tt.want), got, tt.want)
			}
			for i, g := range got {
				if g != tt.want[i] {
					t.Errorf("island[%d]: got %+v, want %+v", i, g, tt.want[i])
				}
			}
		})
	}
}
