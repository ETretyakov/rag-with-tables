# Table RAG

A RAG system for tabular data (CSV/XLSX). Extracts tables, stores them as Parquet in S3, indexes chunks in Qdrant (hybrid dense+sparse search), and answers questions via NL→SQL over DuckDB or synthesis from text chunks.

## Architecture

```
POST /files  →  Island Detection  →  Schema Normalize  →  Parquet → S3
                                                      →  HYDE + Row Chunks → Embed → Qdrant

POST /query  →  Embed Query  →  Qdrant Hybrid Search
                                    ├── table hits  →  NL→SQL → DuckDB (read-only) → Synthesize
                                    └── text hits   →  Synthesize from chunks
```

## Publications

A detailed breakdown of the architecture — the ingestion pipeline and query pipeline with diagrams:

- [Как научить LLM работать с таблицами (RU)](docs/publication-ru.md)
- [Teaching LLMs to Work with Tables (EN)](docs/publication-en.md)

## Quick Start

### Requirements

- Docker & Docker Compose
- Go 1.22+
- API keys: Anthropic (LLM) and VoyageAI (embeddings), or a local OpenAI-compatible server

### 1. Start infrastructure

```bash
docker-compose up -d
```

Starts: **Qdrant** (`:6333`) and **MinIO** (`:9000`, console `:9001`).

### 2. Configuration

Copy the example and fill in the keys:

```bash
cp .env.example .env
```

Minimum set of variables:

```env
# LLM
LLM_PROVIDER=anthropic
LLM_ANTHROPIC_API_KEY=sk-ant-...
LLM_ANTHROPIC_MODEL=claude-3-5-haiku-20241022

# Embeddings
EMBEDDING_PROVIDER=voyageai
EMBEDDING_VOYAGEAI_API_KEY=pa-...
EMBEDDING_VOYAGEAI_MODEL=voyage-3
EMBEDDING_DIMS=1024

# S3 / MinIO (dev defaults)
S3_ENDPOINT=http://localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_BUCKET=table-rags
S3_USE_PATH_STYLE=true

# Qdrant (dev defaults)
QDRANT_HOST=localhost
QDRANT_PORT=6333
QDRANT_COLLECTION=table-rags
```

For local models via Ollama/vLLM:

```env
LLM_PROVIDER=openaicompat
LLM_OPENAICOMPAT_BASE_URL=http://localhost:11434
LLM_OPENAICOMPAT_MODEL=llama3.2

EMBEDDING_PROVIDER=openaicompat
EMBEDDING_OPENAICOMPAT_BASE_URL=http://localhost:11434
EMBEDDING_OPENAICOMPAT_MODEL=nomic-embed-text
EMBEDDING_DIMS=768
```

### 3. Run the service

```bash
go run ./cmd/server
```

Or via Docker:

```bash
docker build -t table-rags .
docker run --env-file .env --network host table-rags
```

Swagger UI: [http://localhost:8080/docs](http://localhost:8080/docs)

## API

### Upload a file

```bash
curl -X POST http://localhost:8080/files \
  -F "file=@data.xlsx"
```

Response: `FileRecord` with `id`, `tables_found`, `chunks_count`, `s3_keys`, `tables`.

### List files

```bash
curl http://localhost:8080/files
```

### File table schema

```bash
curl http://localhost:8080/files/{id}/schema
```

### Delete a file

```bash
curl -X DELETE http://localhost:8080/files/{id}
```

Removes data from Qdrant and Parquet files from S3.

### Query (buffered)

```bash
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"query": "What is the average revenue by region?", "top_k": 10}'
```

Response:

```json
{
  "answer": "Average revenue by region...",
  "sources": [{"source_file": "sales.xlsx", "s3_key": "...", "score": 0.92}],
  "sql_used": "SELECT region, AVG(revenue) FROM data GROUP BY region"
}
```

### Query (Server-Sent Events)

```bash
curl -X POST http://localhost:8080/query/stream \
  -H "Content-Type: application/json" \
  -d '{"query": "Top 5 products by sales volume"}' \
  --no-buffer
```

Event format:

```
data: {"type":"token","content":"Top"}
data: {"type":"token","content":" 5 products..."}
data: {"type":"result","answer":"...","sources":[...],"sql_used":"..."}
```

### Metrics

```bash
curl http://localhost:8080/metrics
```

```json
{
  "ingestion": {"files_processed":5,"tables_processed":12,"chunks_created":3421,"total_duration_ms":8432},
  "query":     {"requests_total":42,"total_duration_ms":12300,"sql_path":31,"text_path":11},
  "duckdb_cache": {"hits":28,"misses":3}
}
```

## Directory Structure

```
cmd/server/          — entry point
internal/
  api/               — Huma + chi handlers (/files, /query, /query/stream, /metrics)
  app/               — dependency initialization, graceful shutdown
  chunker/           — HYDE generation + row chunking
  config/            — YAML + env configuration
  embedder/          — batch dense embedding + BM25 sparse
  ingest/            — CSV/XLSX reader, island detection, schema normalization
  metrics/           — in-process atomic counters
  provider/          — LLM and embedding providers (Anthropic, VoyageAI, OpenAI-compat)
  query/             — pipeline: search → NL→SQL / synthesis + SQL validation
  storage/
    duckdb/          — Parquet→DuckDB loader with TTL cache (read-only connections)
    s3/              — MinIO/S3 client + file catalog
  vectordb/qdrant/   — Qdrant REST client (upsert, hybrid search, delete)
docs/publication-ru.md — article about the system (RU)
docs/publication-en.md — article about the system (EN)
docker-compose.yml   — Qdrant + MinIO
```

## DuckDB Security

All SQL queries generated by the LLM pass through two layers of protection:

1. **Validation** (`query.ValidateSQL`): only `SELECT` is allowed; `DROP`, `DELETE`, `INSERT`, `CREATE`, `COPY`, `ATTACH`, `read_parquet`, `read_csv`, and other dangerous constructs are blocked.
2. **Read-only connection**: DuckDB is opened with `access_mode=read_only` — the engine itself blocks any mutations at the transaction level.
