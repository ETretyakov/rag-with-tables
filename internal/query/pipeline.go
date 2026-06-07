package query

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ETretyakov/rag-with-tables/internal/embedder"
	"github.com/ETretyakov/rag-with-tables/internal/metrics"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
	duckdbcache "github.com/ETretyakov/rag-with-tables/internal/storage/duckdb"
	"github.com/ETretyakov/rag-with-tables/internal/vectordb/qdrant"
)

const (
	defaultTopK     = 10
	searchFetchMult = 5 // fetch this multiple of topK so hits from all tables are candidates
	maxHitsPerTable = 3 // max hits per unique s3_key in the user-visible diverse result
)

// Pipeline orchestrates the full query flow:
// embed query → hybrid search → NL→SQL (with DuckDB) or text synthesis.
type Pipeline struct {
	qdrant *qdrant.Client
	embed  provider.EmbeddingProvider
	llm    provider.LLMProvider    // nil → text synthesis skipped, raw chunks returned
	cache  *duckdbcache.TableCache // nil → SQL path disabled
	log    *slog.Logger
}

// New creates a Pipeline. llm and cache may be nil (degraded text-only mode).
func New(
	q *qdrant.Client,
	embed provider.EmbeddingProvider,
	llm provider.LLMProvider,
	cache *duckdbcache.TableCache,
	log *slog.Logger,
) *Pipeline {
	return &Pipeline{qdrant: q, embed: embed, llm: llm, cache: cache, log: log}
}

// Query executes the full query pipeline for a user question.
func (p *Pipeline) Query(ctx context.Context, req QueryRequest) (*QueryResult, error) {
	start := time.Now()
	metrics.Global.QueryTotal.Add(1)

	result, err := p.run(ctx, req, nil)

	metrics.Global.QueryDurationMs.Add(time.Since(start).Milliseconds())
	return result, err
}

// StreamQuery runs the pipeline and streams the final synthesis via yield.
// Sources and SQLUsed are returned in the QueryResult after streaming completes.
func (p *Pipeline) StreamQuery(ctx context.Context, req QueryRequest, yield func(token string)) (*QueryResult, error) {
	start := time.Now()
	metrics.Global.QueryTotal.Add(1)

	result, err := p.run(ctx, req, yield)

	metrics.Global.QueryDurationMs.Add(time.Since(start).Milliseconds())
	return result, err
}

// run is the shared implementation for Query and StreamQuery.
// When yield is non-nil the final synthesis step streams tokens; otherwise it buffers.
func (p *Pipeline) run(ctx context.Context, req QueryRequest, yield func(token string)) (*QueryResult, error) {
	if req.TopK <= 0 {
		req.TopK = defaultTopK
	}

	// 1. Embed the query (dense).
	vecs, err := p.embed.Embed(ctx, []string{req.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// 2. Compute sparse BM25 vector.
	spVec := embedder.BM25Sparse(req.Query)

	// 3. Hybrid search — fetch more candidates than needed to surface hits from
	// multiple tables even when one table dominates the score ranking.
	allHits, err := p.qdrant.HybridSearch(ctx, vecs[0], spVec, req.TopK*searchFetchMult)
	if err != nil {
		return nil, fmt.Errorf("qdrant search: %w", err)
	}
	if len(allHits) == 0 {
		return &QueryResult{Answer: "No relevant data found.", Sources: []Source{}}, nil
	}

	// 4. Diversify: keep at most maxHitsPerTable hits per unique table, preserve
	// score ordering, cap total at topK for the user-visible sources.
	hits := diversifyHits(allHits, req.TopK, maxHitsPerTable)
	sources := hitsToSources(hits)

	// 5. If we have tables, try the NL→SQL path across unique tables.
	var sqlAttempts []SQLAttempt
	if hits[0].S3Key != "" && p.llm != nil && p.cache != nil {
		sqlResult, attempts, sqlErr := p.sqlPathMulti(ctx, req.Query, allHits, yield)
		sqlAttempts = attempts
		if sqlErr != nil {
			p.log.Warn("sql path failed, falling back to text synthesis", "err", sqlErr)
		} else {
			sqlResult.Sources = sources
			sqlResult.SQLAttempts = sqlAttempts
			metrics.Global.QuerySQLPath.Add(1)
			return sqlResult, nil
		}
	}

	metrics.Global.QueryTextPath.Add(1)

	// 6. Text synthesis (or raw chunks if no LLM).
	if p.llm == nil {
		return &QueryResult{Answer: rawChunksText(hits), Sources: sources, SQLAttempts: sqlAttempts}, nil
	}

	answer, err := p.synthesize(ctx, req.Query, hits, yield)
	if err != nil {
		return nil, fmt.Errorf("synthesize: %w", err)
	}
	return &QueryResult{Answer: answer, Sources: sources, SQLAttempts: sqlAttempts}, nil
}

// diversifyHits returns at most topK hits with no more than maxPerTable hits per
// unique s3_key. The input slice must be sorted by descending score (as Qdrant
// returns it); the output preserves that relative ordering.
func diversifyHits(hits []qdrant.SearchHit, topK, maxPerTable int) []qdrant.SearchHit {
	counts := make(map[string]int)
	result := make([]qdrant.SearchHit, 0, topK)
	for _, h := range hits {
		if len(result) >= topK {
			break
		}
		if counts[h.S3Key] < maxPerTable {
			result = append(result, h)
			counts[h.S3Key]++
		}
	}
	return result
}

// uniqueTableHits returns one representative SearchHit per unique s3_key, ordered
// by the best score seen for that table (descending). Used to drive SQL retries.
func uniqueTableHits(hits []qdrant.SearchHit) []qdrant.SearchHit {
	seen := make(map[string]bool)
	result := make([]qdrant.SearchHit, 0)
	for _, h := range hits {
		if h.S3Key == "" || seen[h.S3Key] {
			continue
		}
		seen[h.S3Key] = true
		result = append(result, h)
	}
	return result
}

const maxSQLSampleRows = 5

// sqlPathMulti runs the NL→SQL path for every unique table found in allHits.
// It returns the synthesised answer together with ALL SQL attempts that were
// executed (regardless of whether they produced rows), so callers can include
// the full audit trail in the response.
//
// Returns (nil, attempts, err) when no table produced a non-empty result;
// the caller should fall through to text synthesis but still attach attempts.
func (p *Pipeline) sqlPathMulti(
	ctx context.Context,
	question string,
	allHits []qdrant.SearchHit,
	yield func(token string),
) (result *QueryResult, attempts []SQLAttempt, err error) {
	tables := uniqueTableHits(allHits)
	attempts = make([]SQLAttempt, 0)
	var lastErr error

	for _, tableHit := range tables {
		if len(tableHit.Schema) == 0 {
			continue
		}

		// Collect sample rows for this table.
		sampleRows := make([]string, 0, maxSQLSampleRows)
		for _, h := range allHits {
			if h.S3Key == tableHit.S3Key && h.Text != "" {
				sampleRows = append(sampleRows, h.Text)
				if len(sampleRows) >= maxSQLSampleRows {
					break
				}
			}
		}

		sqlStr, relevant, genErr := GenerateSQL(ctx, p.llm, question, tableHit.Schema, sampleRows)
		if genErr != nil {
			lastErr = genErr
			p.log.Warn("sql generation failed", "s3_key", tableHit.S3Key, "err", genErr)
			continue
		}
		if !relevant {
			p.log.Debug("table skipped as irrelevant", "s3_key", tableHit.S3Key)
			continue
		}

		if valErr := ValidateSQL(sqlStr); valErr != nil {
			lastErr = valErr
			p.log.Warn("generated sql rejected", "s3_key", tableHit.S3Key, "err", valErr)
			continue
		}

		rows, queryErr := p.cache.Query(ctx, tableHit.S3Key, sqlStr)
		if queryErr != nil {
			lastErr = queryErr
			p.log.Warn("sql execution failed", "s3_key", tableHit.S3Key, "err", queryErr)
			continue
		}

		attempts = append(attempts, SQLAttempt{
			SQL:        sqlStr,
			Rows:       rows,
			SourceFile: tableHit.SourceFile,
			SheetName:  tableHit.SheetName,
			TableIndex: tableHit.TableIndex,
			S3Key:      tableHit.S3Key,
		})

		if len(rows) == 0 {
			p.log.Debug("sql returned no rows", "s3_key", tableHit.S3Key)
			continue
		}

		answer, synthErr := p.synthesizeSQL(ctx, question, sqlStr, tableHit.Summary, rows, yield)
		if synthErr != nil {
			return nil, attempts, synthErr
		}
		return &QueryResult{Answer: answer}, attempts, nil
	}

	if lastErr != nil {
		return nil, attempts, lastErr
	}
	return nil, attempts, fmt.Errorf("no relevant table found for this question")
}

// synthesize picks streaming or buffered synthesis based on provider capability and yield presence.
func (p *Pipeline) synthesize(ctx context.Context, question string, hits []qdrant.SearchHit, yield func(token string)) (string, error) {
	if yield != nil {
		if sp, ok := p.llm.(provider.StreamLLMProvider); ok {
			var buf strings.Builder
			if err := sp.StreamComplete(ctx, synthesizeChunksMessages(question, hits), func(tok string) {
				buf.WriteString(tok)
				yield(tok)
			}); err != nil {
				return "", fmt.Errorf("stream synthesize chunks: %w", err)
			}
			return strings.TrimSpace(buf.String()), nil
		}
	}
	return SynthesizeFromChunks(ctx, p.llm, question, hits)
}

func (p *Pipeline) synthesizeSQL(ctx context.Context, question, sqlStr, summary string, rows []map[string]any, yield func(token string)) (string, error) {
	if yield != nil {
		if sp, ok := p.llm.(provider.StreamLLMProvider); ok {
			var buf strings.Builder
			if err := sp.StreamComplete(ctx, synthesizeSQLMessages(question, sqlStr, summary, rows), func(tok string) {
				buf.WriteString(tok)
				yield(tok)
			}); err != nil {
				return "", fmt.Errorf("stream synthesize sql: %w", err)
			}
			return strings.TrimSpace(buf.String()), nil
		}
	}
	return SynthesizeFromSQL(ctx, p.llm, question, sqlStr, summary, rows)
}

func hitsToSources(hits []qdrant.SearchHit) []Source {
	sources := make([]Source, len(hits))
	for i, h := range hits {
		sources[i] = Source{
			SourceFile: h.SourceFile,
			SheetName:  h.SheetName,
			TableIndex: h.TableIndex,
			RowIndex:   h.RowIndex,
			S3Key:      h.S3Key,
			Score:      h.Score,
			Text:       h.Text,
		}
	}
	return sources
}

func rawChunksText(hits []qdrant.SearchHit) string {
	const maxRaw = 3
	var sb strings.Builder
	for i, h := range hits {
		if i >= maxRaw {
			break
		}
		sb.WriteString(h.Text)
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}
