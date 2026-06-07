package chunker

import (
	"context"
	"fmt"
	"strings"

	"github.com/ETretyakov/rag-with-tables/internal/ingest"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
)

const (
	hydeMaxSampleRows = 20
	hydeSystemPrompt  = `You are a data analyst. Given a description of a data table, ` +
		`generate exactly 5 specific, concise questions that users might ask ` +
		`that this table can answer. Focus on the column names and actual data values. ` +
		`Return only the questions, one per line, without numbering or bullet points.`
)

// GenerateHYDE asks the LLM for hypothetical questions about the table.
// Returns an empty string (no error) when llm is nil — allows offline operation.
func GenerateHYDE(ctx context.Context, table ingest.NormalizedTable, llm provider.LLMProvider) (string, error) {
	if llm == nil {
		return "", nil
	}

	userMsg := buildHYDEPrompt(table)
	result, err := llm.Complete(ctx, []provider.Message{
		{Role: provider.RoleSystem, Content: hydeSystemPrompt},
		{Role: provider.RoleUser, Content: userMsg},
	})
	if err != nil {
		return "", fmt.Errorf("hyde [%s/%s]: %w", table.SourceFile, table.SheetName, err)
	}
	return strings.TrimSpace(result), nil
}

// GenerateHYDEBatch generates HYDE for multiple tables concurrently.
// Results are returned in the same order as tables.
// concurrency ≤ 0 defaults to 4.
func GenerateHYDEBatch(
	ctx context.Context,
	tables []ingest.NormalizedTable,
	llm provider.LLMProvider,
	concurrency int,
) ([]string, error) {
	if concurrency <= 0 {
		concurrency = 4
	}

	type result struct {
		idx  int
		hyde string
		err  error
	}

	sem := make(chan struct{}, concurrency)
	ch := make(chan result, len(tables))

	for i, t := range tables {
		sem <- struct{}{}
		go func(idx int, tbl ingest.NormalizedTable) {
			defer func() { <-sem }()
			h, err := GenerateHYDE(ctx, tbl, llm)
			ch <- result{idx: idx, hyde: h, err: err}
		}(i, t)
	}

	hydes := make([]string, len(tables))
	for range tables {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		hydes[r.idx] = r.hyde
	}
	return hydes, nil
}

// buildHYDEPrompt constructs the user message with table schema + sample rows.
func buildHYDEPrompt(table ingest.NormalizedTable) string {
	var sb strings.Builder

	// Table identity
	sb.WriteString("Table: ")
	sb.WriteString(table.SourceFile)
	if table.SheetName != "" {
		sb.WriteString(" / ")
		sb.WriteString(table.SheetName)
	}
	sb.WriteByte('\n')

	// Column list: "OriginalName (TYPE), ..."
	colDescs := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		name := c.OriginalName
		if name == "" {
			name = c.Name
		}
		colDescs[i] = fmt.Sprintf("%s (%s)", name, c.Type)
	}
	sb.WriteString("Columns: ")
	sb.WriteString(strings.Join(colDescs, ", "))
	sb.WriteString("\n\nSample data:\n")

	// First N non-empty rows
	n := min(hydeMaxSampleRows, len(table.Rows))
	for i := range n {
		line := formatRow(table.Rows[i], table.Columns)
		if line != "" {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}
