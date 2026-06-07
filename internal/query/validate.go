package query

import (
	"fmt"
	"strings"
)

// mutatingKeywords are SQL tokens that can modify data or schema.
// This blocklist is a defense-in-depth layer on top of the read-only DuckDB connection.
var mutatingKeywords = []string{
	"drop ", "delete ", "insert ", "update ", "create ", "alter ",
	"truncate ", "replace ", "merge ", "upsert ",
	// DDL / admin
	"copy ", "attach ", "detach ", "pragma ", "load ", "install ",
	"export ", "import ",
	// DuckDB-specific table functions that read from filesystem / network
	"read_csv", "read_json", "read_ndjson", "read_parquet",
	"httpfs", "s3://", "http://", "https://",
	// Comment-based bypass attempts
	"/*", "*/",
}

// ValidateSQL returns an error if sqlStr is not a safe SELECT-only query.
// It is intentionally strict: any doubt → reject.
func ValidateSQL(sqlStr string) error {
	normalized := strings.TrimSpace(strings.ToLower(sqlStr))

	if normalized == "" {
		return fmt.Errorf("empty query")
	}

	// Must start with SELECT (including leading comments after normalization).
	if !strings.HasPrefix(normalized, "select") {
		return fmt.Errorf("only SELECT queries are allowed")
	}

	for _, kw := range mutatingKeywords {
		if strings.Contains(normalized, kw) {
			return fmt.Errorf("disallowed token in query: %q", strings.TrimSpace(kw))
		}
	}
	return nil
}
