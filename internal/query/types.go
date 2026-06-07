package query

// QueryRequest is the input for the query pipeline.
type QueryRequest struct {
	Query string
	TopK  int
}

// Source describes a chunk that contributed to the answer.
type Source struct {
	SourceFile string  `json:"source_file"`
	SheetName  string  `json:"sheet_name,omitempty"`
	TableIndex int     `json:"table_index"`
	RowIndex   int     `json:"row_index"`
	S3Key      string  `json:"s3_key"`
	Score      float64 `json:"score"`
	Text       string  `json:"text,omitempty"`
}

// SQLAttempt holds one SQL query that was executed and its raw result rows.
type SQLAttempt struct {
	SQL        string           `json:"sql"`
	Rows       []map[string]any `json:"rows"`
	SourceFile string           `json:"source_file,omitempty"`
	SheetName  string           `json:"sheet_name,omitempty"`
	TableIndex int              `json:"table_index"`
	S3Key      string           `json:"s3_key,omitempty"`
}

// QueryResult is the output of the query pipeline.
type QueryResult struct {
	Answer      string       `json:"answer"`
	Sources     []Source     `json:"sources"`
	SQLAttempts []SQLAttempt `json:"sql_attempts,omitempty"`
}
