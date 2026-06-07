package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/ETretyakov/rag-with-tables/internal/chunker"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
)

// GenerateSQL asks the LLM whether this table can answer the question and, if so,
// produces a DuckDB SELECT query. The second return value is false when the model
// determines the table is not relevant (it responds with the sentinel "SKIP").
//
// sampleRows should contain a few formatted row strings (e.g. from hit texts)
// so the model can see the actual data values and infer their language.
func GenerateSQL(
	ctx context.Context,
	llm provider.LLMProvider,
	question string,
	schema []chunker.ColumnInfo,
	sampleRows []string,
) (sql string, relevant bool, err error) {
	schemaLines := make([]string, len(schema))
	for i, col := range schema {
		orig := col.OriginalName
		if orig == "" || orig == col.Name {
			schemaLines[i] = fmt.Sprintf("  - %s (%s)", col.Name, col.Type)
		} else {
			schemaLines[i] = fmt.Sprintf("  - %s  [original: %q]  (%s)", col.Name, orig, col.Type)
		}
	}

	sampleSection := ""
	if len(sampleRows) > 0 {
		sampleSection = "\nSample data rows:\n"
		for _, r := range sampleRows {
			sampleSection += "  " + r + "\n"
		}
	}

	prompt := fmt.Sprintf(`You are a SQL expert. Your task is to decide whether the table described below can answer the question, and if so, to generate a DuckDB SQL SELECT query.

Table name: data
Schema:
%s
%s
Question: %s

Instructions:
- First decide: does this table contain data that can answer the question?
- If NO — respond with exactly one word: SKIP
- If YES — respond with ONLY the raw SQL SELECT query: no markdown fences, no explanation, no comments, no semicolon at the end.
- Use column names exactly as listed in the schema (snake_case names).
- Only SELECT — never DROP, DELETE, INSERT, UPDATE, CREATE, ALTER, COPY.
- Look at the sample rows to understand what language and format the values are stored in. Use that same language in WHERE / LIKE patterns, regardless of the language of the question.`,
		strings.Join(schemaLines, "\n"),
		sampleSection,
		question,
	)

	raw, err2 := llm.Complete(ctx, []provider.Message{
		{Role: provider.RoleUser, Content: prompt},
	})
	if err2 != nil {
		return "", false, fmt.Errorf("generate sql: %w", err2)
	}

	cleaned := cleanSQL(raw)
	if strings.EqualFold(cleaned, "skip") {
		return "", false, nil
	}
	return cleaned, true, nil
}

// cleanSQL strips markdown code fences and trailing semicolons that LLMs sometimes add.
func cleanSQL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSuffix(strings.TrimSpace(s), ";")
}
