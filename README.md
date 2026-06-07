# Table RAG

RAG-система для табличных данных (CSV/XLSX). Извлекает таблицы, сохраняет их как Parquet в S3, индексирует чанки в Qdrant (hybrid dense+sparse search) и отвечает на вопросы через NL→SQL поверх DuckDB или синтез из текстовых чанков.

## Архитектура

```
POST /files  →  Island Detection  →  Schema Normalize  →  Parquet → S3
                                                      →  HYDE + Row Chunks → Embed → Qdrant

POST /query  →  Embed Query  →  Qdrant Hybrid Search
                                    ├── table hits  →  NL→SQL → DuckDB (read-only) → Synthesize
                                    └── text hits   →  Synthesize from chunks
```

## Публикации

Подробный разбор архитектуры — пайплайн загрузки данных и пайплайн обработки запросов со схемами:

- [Как научить LLM работать с таблицами (RU)](docs/publication-ru.md)
- [Teaching LLMs to Work with Tables (EN)](docs/publication-en.md)

## Быстрый старт

### Требования

- Docker & Docker Compose
- Go 1.22+
- API-ключи: Anthropic (LLM) и VoyageAI (embeddings), либо локальный OpenAI-совместимый сервер

### 1. Запуск инфраструктуры

```bash
docker-compose up -d
```

Поднимает: **Qdrant** (`:6333`) и **MinIO** (`:9000`, консоль `:9001`).

### 2. Конфигурация

Скопируйте пример и заполните ключи:

```bash
cp .env.example .env
```

Минимальный набор переменных:

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

Для локальных моделей через Ollama/vLLM:

```env
LLM_PROVIDER=openaicompat
LLM_OPENAICOMPAT_BASE_URL=http://localhost:11434
LLM_OPENAICOMPAT_MODEL=llama3.2

EMBEDDING_PROVIDER=openaicompat
EMBEDDING_OPENAICOMPAT_BASE_URL=http://localhost:11434
EMBEDDING_OPENAICOMPAT_MODEL=nomic-embed-text
EMBEDDING_DIMS=768
```

### 3. Запуск сервиса

```bash
go run ./cmd/server
```

Или через Docker:

```bash
docker build -t table-rags .
docker run --env-file .env --network host table-rags
```

Swagger UI: [http://localhost:8080/docs](http://localhost:8080/docs)

## API

### Загрузка файла

```bash
curl -X POST http://localhost:8080/files \
  -F "file=@data.xlsx"
```

Ответ: `FileRecord` с `id`, `tables_found`, `chunks_count`, `s3_keys`, `tables`.

### Список файлов

```bash
curl http://localhost:8080/files
```

### Схема таблиц файла

```bash
curl http://localhost:8080/files/{id}/schema
```

### Удаление файла

```bash
curl -X DELETE http://localhost:8080/files/{id}
```

Удаляет данные из Qdrant и Parquet-файлы из S3.

### Запрос (буферизованный)

```bash
curl -X POST http://localhost:8080/query \
  -H "Content-Type: application/json" \
  -d '{"query": "Какова средняя выручка по регионам?", "top_k": 10}'
```

Ответ:

```json
{
  "answer": "Средняя выручка по регионам...",
  "sources": [{"source_file": "sales.xlsx", "s3_key": "...", "score": 0.92}],
  "sql_used": "SELECT region, AVG(revenue) FROM data GROUP BY region"
}
```

### Запрос (Server-Sent Events)

```bash
curl -X POST http://localhost:8080/query/stream \
  -H "Content-Type: application/json" \
  -d '{"query": "Топ-5 продуктов по объёму продаж"}' \
  --no-buffer
```

Формат событий:

```
data: {"type":"token","content":"Топ-5"}
data: {"type":"token","content":" продуктов..."}
data: {"type":"result","answer":"...","sources":[...],"sql_used":"..."}
```

### Метрики

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

## Структура директорий

```
cmd/server/          — точка входа
internal/
  api/               — Huma + chi handlers (/files, /query, /query/stream, /metrics)
  app/               — инициализация зависимостей, graceful shutdown
  chunker/           — HYDE-генерация + row chunking
  config/            — YAML + env конфигурация
  embedder/          — batch dense embedding + BM25 sparse
  ingest/            — CSV/XLSX reader, island detection, schema normalization
  metrics/           — in-process atomic counters
  provider/          — LLM и embedding провайдеры (Anthropic, VoyageAI, OpenAI-compat)
  query/             — pipeline: search → NL→SQL / synthesis + SQL validation
  storage/
    duckdb/          — Parquet→DuckDB loader с TTL-кэшем (read-only connections)
    s3/              — MinIO/S3 клиент + каталог файлов
  vectordb/qdrant/   — REST-клиент Qdrant (upsert, hybrid search, delete)
docs/publication-ru.md — статья о системе (RU)
docs/publication-en.md — статья о системе (EN)
docker-compose.yml   — Qdrant + MinIO
```

## Безопасность DuckDB

Все SQL-запросы, сгенерированные LLM, проходят два уровня защиты:

1. **Валидация** (`query.ValidateSQL`): только `SELECT`, блокируются `DROP`, `DELETE`, `INSERT`, `CREATE`, `COPY`, `ATTACH`, `read_parquet`, `read_csv` и другие опасные конструкции.
2. **Read-only соединение**: DuckDB открывается с `access_mode=read_only` — движок сам блокирует любые мутации на уровне транзакций.
