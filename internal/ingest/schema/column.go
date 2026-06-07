package schema

import (
	"strconv"
	"strings"
	"time"
)

// ColumnType represents a DuckDB column data type.
type ColumnType string

const (
	TypeBoolean   ColumnType = "BOOLEAN"
	TypeInteger   ColumnType = "INTEGER"
	TypeDouble    ColumnType = "DOUBLE"
	TypeDate      ColumnType = "DATE"
	TypeTimestamp ColumnType = "TIMESTAMP"
	TypeVarchar   ColumnType = "VARCHAR"
)

// ColumnDef describes one column of a normalized table.
type ColumnDef struct {
	Index        int
	Name         string     // normalized snake_case name
	OriginalName string     // raw value from the header row
	Type         ColumnType
}

// ParseValue converts the raw cell string to the appropriate Go type for
// database/sql insertion. Returns nil for empty or unparseable values.
func (c ColumnDef) ParseValue(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	switch c.Type {
	case TypeBoolean:
		l := strings.ToLower(s)
		return boolTrueSet[l]
	case TypeInteger:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil
		}
		return v
	case TypeDouble:
		v, err := strconv.ParseFloat(normNumeric(s), 64)
		if err != nil {
			return nil
		}
		return v
	case TypeDate:
		for _, f := range DateFormats {
			if t, err := time.Parse(f, s); err == nil {
				return t
			}
		}
		return nil
	case TypeTimestamp:
		for _, f := range TimestampFormats {
			if t, err := time.Parse(f, s); err == nil {
				return t
			}
		}
		return nil
	default:
		return s
	}
}

