package reader

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ETretyakov/rag-with-tables/internal/ingest"
)

// Reader extracts raw tables from any io.Reader source.
// The name parameter carries the original filename — used only for metadata
// (SourceFile field) and format detection; no actual file path is needed.
type Reader interface {
	Extract(ctx context.Context, r io.Reader, name string) ([]ingest.RawTable, error)
}

// New returns the appropriate Reader based on the file extension in name.
func New(name string) (Reader, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv":
		return &csvReader{}, nil
	case ".xlsx":
		return &xlsxReader{}, nil
	default:
		return nil, fmt.Errorf("unsupported file format %q (supported: .csv, .xlsx)", filepath.Ext(name))
	}
}
