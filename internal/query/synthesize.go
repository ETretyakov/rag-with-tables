package query

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ETretyakov/rag-with-tables/internal/provider"
	"github.com/ETretyakov/rag-with-tables/internal/vectordb/qdrant"
)

// SynthesizeFromSQL generates a natural-language answer from SQL query results.
// summary is the table's LLM-generated description; pass empty string if unavailable.
func SynthesizeFromSQL(
	ctx context.Context,
	llm provider.LLMProvider,
	question string,
	sqlStr string,
	summary string,
	rows []map[string]any,
) (string, error) {
	answer, err := llm.Complete(ctx, synthesizeSQLMessages(question, sqlStr, summary, rows))
	if err != nil {
		return "", fmt.Errorf("synthesize from sql: %w", err)
	}
	return strings.TrimSpace(answer), nil
}

// SynthesizeFromChunks generates an answer directly from the text of search hits.
func SynthesizeFromChunks(
	ctx context.Context,
	llm provider.LLMProvider,
	question string,
	hits []qdrant.SearchHit,
) (string, error) {
	answer, err := llm.Complete(ctx, synthesizeChunksMessages(question, hits))
	if err != nil {
		return "", fmt.Errorf("synthesize from chunks: %w", err)
	}
	return strings.TrimSpace(answer), nil
}

// synthesizeSQLMessages builds the prompt messages for SQL-result synthesis.
// summary is the LLM-generated table description; it is prepended when non-empty.
func synthesizeSQLMessages(question, sqlStr, summary string, rows []map[string]any) []provider.Message {
	var sb strings.Builder
	if summary != "" {
		sb.WriteString("Table description:\n")
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}
	fmt.Fprintf(&sb, `Answer the following question based on the query results below.

Question: %s

SQL query executed:
%s

Results:
%s

Provide a clear, concise answer in natural language.`,
		question, sqlStr, formatRows(rows),
	)
	return []provider.Message{{Role: provider.RoleUser, Content: sb.String()}}
}

// synthesizeChunksMessages builds the prompt messages for chunk-text synthesis.
// Table summaries (one per unique table from hits) are prepended to the context.
func synthesizeChunksMessages(question string, hits []qdrant.SearchHit) []provider.Message {
	const maxHits = 5

	// Collect one summary per unique table (preserving hit order).
	seenTable := map[string]bool{}
	var summaryLines []string
	for _, h := range hits {
		if seenTable[h.S3Key] || h.Summary == "" {
			continue
		}
		seenTable[h.S3Key] = true
		label := h.SourceFile
		if h.SheetName != "" {
			label += "/" + h.SheetName
		}
		summaryLines = append(summaryLines, fmt.Sprintf("[%s]: %s", label, h.Summary))
	}

	var sb strings.Builder
	if len(summaryLines) > 0 {
		sb.WriteString("Table descriptions:\n")
		sb.WriteString(strings.Join(summaryLines, "\n"))
		sb.WriteString("\n\n")
	}

	rowLines := make([]string, 0, maxHits)
	for i, h := range hits {
		if i >= maxHits {
			break
		}
		rowLines = append(rowLines, fmt.Sprintf("[%d] %s", i+1, h.Text))
	}

	fmt.Fprintf(&sb, `Answer the following question based on the context below.

Question: %s

Context (table rows):
%s

Provide a clear, concise answer.`,
		question, strings.Join(rowLines, "\n"),
	)
	return []provider.Message{{Role: provider.RoleUser, Content: sb.String()}}
}

func formatRows(rows []map[string]any) string {
	if len(rows) == 0 {
		return "(no results)"
	}

	// Stable column order.
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	header := strings.Join(cols, " | ")
	sep := strings.Repeat("-", len(header))

	lines := make([]string, 0, len(rows)+2)
	lines = append(lines, header, sep)
	for _, row := range rows {
		vals := make([]string, len(cols))
		for i, col := range cols {
			vals[i] = fmt.Sprintf("%v", row[col])
		}
		lines = append(lines, strings.Join(vals, " | "))
	}
	return strings.Join(lines, "\n")
}
