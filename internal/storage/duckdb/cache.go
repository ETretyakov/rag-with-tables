package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"time"

	_ "github.com/marcboeker/go-duckdb" // register duckdb driver

	"github.com/ETretyakov/rag-with-tables/internal/config"
	s3client "github.com/ETretyakov/rag-with-tables/internal/storage/s3"
)

type tableEntry struct {
	db       *sql.DB
	dbPath   string // path to the on-disk .duckdb file; deleted on eviction
	lastUsed time.Time
	mu       sync.Mutex
}

// TableCache keeps read-only DuckDB databases loaded from S3 Parquet files.
// Each entry is evicted after ttl of inactivity.
//
// Isolation model:
//   - Data is first written to a temp .duckdb file (read-write).
//   - The file is then reopened with access_mode=read_only so DuckDB itself
//     enforces that no DDL / DML can be executed by subsequent queries.
//   - The caller is additionally responsible for validating SQL before calling Query.
type TableCache struct {
	entries sync.Map // string(s3key) → *tableEntry
	s3      *s3client.Client
	ttl     time.Duration
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

func NewTableCache(s3 *s3client.Client, cfg config.DuckDBConfig) *TableCache {
	c := &TableCache{
		s3:     s3,
		ttl:    cfg.CacheTTL,
		stopCh: make(chan struct{}),
	}
	c.wg.Add(1)
	go c.evictLoop()
	return c
}

// Close stops the eviction goroutine and releases all cached databases.
func (c *TableCache) Close() {
	close(c.stopCh)
	c.wg.Wait()
	c.entries.Range(func(k, v any) bool {
		e := v.(*tableEntry)
		e.db.Close()
		os.Remove(e.dbPath)
		return true
	})
}

func (c *TableCache) evictLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.evict()
		case <-c.stopCh:
			return
		}
	}
}

func (c *TableCache) evict() {
	now := time.Now()
	c.entries.Range(func(k, v any) bool {
		e := v.(*tableEntry)
		e.mu.Lock()
		expired := now.Sub(e.lastUsed) > c.ttl
		e.mu.Unlock()
		if expired {
			c.entries.Delete(k)
			e.db.Close()
			os.Remove(e.dbPath)
		}
		return true
	})
}

// Query executes sqlStr against the table loaded from s3Key.
// The table is always named "data" inside the DuckDB database.
// The database is opened in read-only mode; any attempt to mutate data or schema
// will be rejected by DuckDB before reaching the query planner.
func (c *TableCache) Query(ctx context.Context, s3Key string, sqlStr string) ([]map[string]any, error) {
	entry, err := c.getOrLoad(ctx, s3Key)
	if err != nil {
		return nil, err
	}

	entry.mu.Lock()
	entry.lastUsed = time.Now()
	entry.mu.Unlock()

	rows, err := entry.db.QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	return scanRows(rows)
}

func (c *TableCache) getOrLoad(ctx context.Context, s3Key string) (*tableEntry, error) {
	if v, ok := c.entries.Load(s3Key); ok {
		return v.(*tableEntry), nil
	}

	entry, err := c.loadFromS3(ctx, s3Key)
	if err != nil {
		return nil, err
	}

	// Handle race: another goroutine may have loaded the same key concurrently.
	actual, loaded := c.entries.LoadOrStore(s3Key, entry)
	if loaded {
		entry.db.Close()
		os.Remove(entry.dbPath)
		return actual.(*tableEntry), nil
	}
	return entry, nil
}

func (c *TableCache) loadFromS3(ctx context.Context, s3Key string) (*tableEntry, error) {
	// 1. Download Parquet bytes.
	data, err := c.s3.Download(ctx, s3Key)
	if err != nil {
		return nil, fmt.Errorf("download parquet %s: %w", s3Key, err)
	}

	// 2. Write Parquet to a temp file so DuckDB read_parquet can access it.
	pqFile, err := os.CreateTemp("", "table-rags-*.parquet")
	if err != nil {
		return nil, fmt.Errorf("create parquet temp file: %w", err)
	}
	pqPath := pqFile.Name()
	if _, err := pqFile.Write(data); err != nil {
		pqFile.Close()
		os.Remove(pqPath)
		return nil, fmt.Errorf("write parquet temp file: %w", err)
	}
	pqFile.Close()
	defer os.Remove(pqPath)

	// 3. Create a temp .duckdb file and load the Parquet data into it (read-write).
	dbFile, err := os.CreateTemp("", "table-rags-*.duckdb")
	if err != nil {
		return nil, fmt.Errorf("create duckdb temp file: %w", err)
	}
	dbPath := dbFile.Name()
	dbFile.Close()
	// Remove the empty file so DuckDB creates a fresh database at that path.
	os.Remove(dbPath)

	rw, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open duckdb (rw): %w", err)
	}
	rw.SetMaxOpenConns(1)

	loadSQL := fmt.Sprintf("CREATE TABLE data AS SELECT * FROM read_parquet('%s')", pqPath)
	if _, err := rw.ExecContext(ctx, loadSQL); err != nil {
		rw.Close()
		os.Remove(dbPath)
		return nil, fmt.Errorf("load parquet into duckdb: %w", err)
	}
	if err := rw.Close(); err != nil {
		os.Remove(dbPath)
		return nil, fmt.Errorf("close rw duckdb: %w", err)
	}

	// 4. Reopen the same file in read-only mode.
	// DuckDB enforces at the engine level that no DDL or DML can execute on this connection.
	ro, err := sql.Open("duckdb", dbPath+"?access_mode=read_only")
	if err != nil {
		os.Remove(dbPath)
		return nil, fmt.Errorf("open duckdb (ro): %w", err)
	}
	ro.SetMaxOpenConns(1)

	return &tableEntry{db: ro, dbPath: dbPath, lastUsed: time.Now()}, nil
}

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
