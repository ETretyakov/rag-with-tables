// Package duckdb provides helpers for converting normalized tables to Parquet.
// Writing uses apache/arrow-go (already a transitive dependency via go-duckdb)
// so no temporary files are created — data flows entirely through memory.
// DuckDB itself is used in later phases for SQL queries over loaded Parquet files.
package duckdb

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/ETretyakov/rag-with-tables/internal/ingest/schema"
)

// epochDay is used to convert time.Time to Arrow Date32 (days since 1970-01-01).
var epochDay = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

// ToParquet serialises cols+rows to Parquet in memory and returns the raw bytes.
// No temporary files are created; the Arrow Parquet writer streams directly to
// a bytes.Buffer. Compression is Snappy (compatible with DuckDB read_parquet).
func ToParquet(_ context.Context, cols []schema.ColumnDef, rows [][]string) ([]byte, error) {
	arrowSchema := buildArrowSchema(cols)
	pool := memory.NewGoAllocator()

	rec, err := buildRecord(pool, arrowSchema, cols, rows)
	if err != nil {
		return nil, fmt.Errorf("build arrow record: %w", err)
	}
	defer rec.Release()

	var buf bytes.Buffer
	props := parquet.NewWriterProperties(
		parquet.WithCompression(compress.Codecs.Snappy),
	)
	w, err := pqarrow.NewFileWriter(arrowSchema, &buf, props,
		pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema()))
	if err != nil {
		return nil, fmt.Errorf("create parquet writer: %w", err)
	}

	if err := w.Write(rec); err != nil {
		return nil, fmt.Errorf("write record: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close parquet writer: %w", err)
	}

	return buf.Bytes(), nil
}

// ── Arrow schema helpers ──────────────────────────────────────────────────────

func buildArrowSchema(cols []schema.ColumnDef) *arrow.Schema {
	fields := make([]arrow.Field, len(cols))
	for i, c := range cols {
		fields[i] = arrow.Field{Name: c.Name, Type: columnTypeToArrow(c.Type), Nullable: true}
	}
	return arrow.NewSchema(fields, nil)
}

func columnTypeToArrow(t schema.ColumnType) arrow.DataType {
	switch t {
	case schema.TypeBoolean:
		return arrow.FixedWidthTypes.Boolean
	case schema.TypeInteger:
		return arrow.PrimitiveTypes.Int64
	case schema.TypeDouble:
		return arrow.PrimitiveTypes.Float64
	case schema.TypeDate:
		return arrow.FixedWidthTypes.Date32
	case schema.TypeTimestamp:
		return &arrow.TimestampType{Unit: arrow.Microsecond, TimeZone: "UTC"}
	default:
		return arrow.BinaryTypes.String
	}
}

// ── Record builder ────────────────────────────────────────────────────────────

func buildRecord(pool memory.Allocator, sc *arrow.Schema, cols []schema.ColumnDef, rows [][]string) (arrow.Record, error) {
	builders := make([]array.Builder, len(cols))
	for i, c := range cols {
		builders[i] = newBuilder(pool, c.Type)
	}
	defer func() {
		for _, b := range builders {
			b.Release()
		}
	}()

	for ri, row := range rows {
		for i, col := range cols {
			raw := ""
			if i < len(row) {
				raw = strings.TrimSpace(row[i])
			}
			if err := appendValue(builders[i], col.Type, raw); err != nil {
				return nil, fmt.Errorf("row %d col %d (%s): %w", ri, i, col.Name, err)
			}
		}
	}

	arrays := make([]arrow.Array, len(builders))
	for i, b := range builders {
		arrays[i] = b.NewArray()
		defer arrays[i].Release()
	}

	return array.NewRecord(sc, arrays, int64(len(rows))), nil
}

func newBuilder(pool memory.Allocator, t schema.ColumnType) array.Builder {
	switch t {
	case schema.TypeBoolean:
		return array.NewBooleanBuilder(pool)
	case schema.TypeInteger:
		return array.NewInt64Builder(pool)
	case schema.TypeDouble:
		return array.NewFloat64Builder(pool)
	case schema.TypeDate:
		return array.NewDate32Builder(pool)
	case schema.TypeTimestamp:
		return array.NewTimestampBuilder(pool, &arrow.TimestampType{Unit: arrow.Microsecond, TimeZone: "UTC"})
	default:
		return array.NewStringBuilder(pool)
	}
}

var boolTrue = map[string]bool{"true": true, "yes": true, "t": true, "y": true}

func appendValue(b array.Builder, t schema.ColumnType, s string) error {
	if s == "" {
		b.AppendNull()
		return nil
	}
	switch t {
	case schema.TypeBoolean:
		b.(*array.BooleanBuilder).Append(boolTrue[strings.ToLower(s)])
	case schema.TypeInteger:
		parsed, ok := parseInt(s)
		if !ok {
			b.AppendNull()
		} else {
			b.(*array.Int64Builder).Append(parsed)
		}
	case schema.TypeDouble:
		parsed, ok := parseFloat(s)
		if !ok {
			b.AppendNull()
		} else {
			b.(*array.Float64Builder).Append(parsed)
		}
	case schema.TypeDate:
		t, ok := parseDate(s)
		if !ok {
			b.AppendNull()
		} else {
			// Date32 = days since Unix epoch.
			d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
			days := arrow.Date32(d.Sub(epochDay) / (24 * time.Hour))
			b.(*array.Date32Builder).Append(days)
		}
	case schema.TypeTimestamp:
		t, ok := parseTimestamp(s)
		if !ok {
			b.AppendNull()
		} else {
			b.(*array.TimestampBuilder).Append(arrow.Timestamp(t.UTC().UnixMicro()))
		}
	default:
		b.(*array.StringBuilder).Append(s)
	}
	return nil
}

// ── parsing helpers ───────────────────────────────────────────────────────────

func parseInt(s string) (int64, bool) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err == nil
}

func parseFloat(s string) (float64, bool) {
	var v float64
	_, err := fmt.Sscanf(normNumeric(s), "%g", &v)
	return v, err == nil
}

// normNumeric mirrors schema.normNumeric: strips currency symbols, percent signs,
// and comma thousands-separators before float parsing.
func normNumeric(s string) string {
	s = strings.TrimLeft(s, " $€£¥₹")
	s = strings.TrimRight(s, "% ")
	s = strings.TrimSpace(s)
	if strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", "")
	}
	return s
}

func parseDate(s string) (time.Time, bool) {
	for _, f := range schema.DateFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseTimestamp(s string) (time.Time, bool) {
	for _, f := range schema.TimestampFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
