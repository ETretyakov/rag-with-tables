package ingest

import "github.com/ETretyakov/rag-with-tables/internal/ingest/schema"

// TableBounds описывает положение таблицы в исходном файле (индексы 0-based).
type TableBounds struct {
	SheetName string
	StartRow  int
	StartCol  int
	EndRow    int // inclusive
	EndCol    int // inclusive
}

// RawTable — сырая таблица, извлечённая из CSV или XLSX до любой обработки.
type RawTable struct {
	SourceFile string
	SheetName  string // пусто для CSV
	TableIndex int    // порядковый номер на листе (0-based)
	Headers    []string
	Rows       [][]string
	Bounds     TableBounds
	Summary    string // LLM-сгенерированное описание таблицы (заполняется до нормализации)
}

// NormalizedTable — RawTable с нормализованной схемой и ссылкой на Parquet в S3.
type NormalizedTable struct {
	RawTable
	Columns  []schema.ColumnDef
	S3Key    string
	RowCount int
}
