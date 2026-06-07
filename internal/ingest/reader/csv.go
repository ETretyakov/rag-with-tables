package reader

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ETretyakov/rag-with-tables/internal/ingest"
)

type csvReader struct{}

func (r *csvReader) Extract(_ context.Context, rd io.Reader, name string) ([]ingest.RawTable, error) {
	cr := csv.NewReader(rd)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	var all [][]string
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv %s: %w", name, err)
		}
		all = append(all, rec)
	}

	if len(all) == 0 {
		return nil, nil
	}

	headers := all[0]
	rows := all[1:]

	return []ingest.RawTable{
		{
			SourceFile: filepath.Base(name),
			SheetName:  "",
			TableIndex: 0,
			Headers:    headers,
			Rows:       rows,
			Bounds: ingest.TableBounds{
				StartRow: 0,
				StartCol: 0,
				EndRow:   len(all) - 1,
				EndCol:   len(headers) - 1,
			},
		},
	}, nil
}
