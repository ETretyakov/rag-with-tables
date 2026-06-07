package chunker

import (
	"context"
	"fmt"
	"strings"

	"github.com/ETretyakov/rag-with-tables/internal/ingest"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
)

const (
	summaryMaxSampleRows = 30
	summarySystemPrompt  = `You are a data analyst. Write a concise summary (3–5 sentences) of the table provided.
Describe: what the table represents, the key columns and their meaning, the approximate number of rows,
and any notable patterns or value ranges visible in the sample data.
Be specific — mention actual column names and example values. Write in English.`
)

// GenerateSummary asks the LLM for a descriptive paragraph about a raw table.
// Returns an empty string (no error) when llm is nil — allows offline operation.
func GenerateSummary(ctx context.Context, table ingest.RawTable, llm provider.LLMProvider) (string, error) {
	if llm == nil {
		return "", nil
	}
	msg := buildSummaryPrompt(table)
	result, err := llm.Complete(ctx, []provider.Message{
		{Role: provider.RoleSystem, Content: summarySystemPrompt},
		{Role: provider.RoleUser, Content: msg},
	})
	if err != nil {
		return "", fmt.Errorf("summary [%s/%s]: %w", table.SourceFile, table.SheetName, err)
	}
	return strings.TrimSpace(result), nil
}

// GenerateSummaryBatch generates summaries for multiple raw tables concurrently.
// Results are returned in the same order as the input slice.
// concurrency ≤ 0 defaults to 4.
func GenerateSummaryBatch(
	ctx context.Context,
	tables []ingest.RawTable,
	llm provider.LLMProvider,
	concurrency int,
) ([]string, error) {
	if concurrency <= 0 {
		concurrency = 4
	}
	if llm == nil {
		return make([]string, len(tables)), nil
	}

	type result struct {
		idx     int
		summary string
		err     error
	}

	sem := make(chan struct{}, concurrency)
	ch := make(chan result, len(tables))

	for i, t := range tables {
		sem <- struct{}{}
		go func(idx int, tbl ingest.RawTable) {
			defer func() { <-sem }()
			s, err := GenerateSummary(ctx, tbl, llm)
			ch <- result{idx: idx, summary: s, err: err}
		}(i, t)
	}

	summaries := make([]string, len(tables))
	for range tables {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		summaries[r.idx] = r.summary
	}
	return summaries, nil
}

func buildSummaryPrompt(table ingest.RawTable) string {
	var sb strings.Builder

	sb.WriteString("Table: ")
	sb.WriteString(table.SourceFile)
	if table.SheetName != "" {
		sb.WriteString(" / ")
		sb.WriteString(table.SheetName)
	}
	fmt.Fprintf(&sb, "\nTotal rows: %d\n", len(table.Rows))
	sb.WriteString("Columns (original names): ")
	sb.WriteString(strings.Join(table.Headers, ", "))
	sb.WriteString("\n\nSample data:\n")
	sb.WriteString(strings.Join(table.Headers, " | "))
	sb.WriteByte('\n')

	n := min(summaryMaxSampleRows, len(table.Rows))
	for i := range n {
		sb.WriteString(strings.Join(table.Rows[i], " | "))
		sb.WriteByte('\n')
	}

	return sb.String()
}
