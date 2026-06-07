package api

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/ETretyakov/rag-with-tables/internal/chunker"
	"github.com/ETretyakov/rag-with-tables/internal/embedder"
	"github.com/ETretyakov/rag-with-tables/internal/ingest"
	"github.com/ETretyakov/rag-with-tables/internal/ingest/reader"
	"github.com/ETretyakov/rag-with-tables/internal/metrics"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
	s3client "github.com/ETretyakov/rag-with-tables/internal/storage/s3"
	qdrantclient "github.com/ETretyakov/rag-with-tables/internal/vectordb/qdrant"
)

// FileDeps carries all dependencies for the /files resource.
type FileDeps struct {
	S3       *s3client.Client
	Qdrant   *qdrantclient.Client
	LLM      provider.LLMProvider       // nil → HYDE skipped
	Embed    provider.EmbeddingProvider // required for upload
	EmbedDim int
	Log      *slog.Logger
}

// ── Huma types ────────────────────────────────────────────────────────────────

type ListFilesOutput struct {
	Body struct {
		Files []s3client.FileRecord `json:"files"`
	}
}

type GetFileOutput struct {
	Body s3client.FileRecord
}

type UploadFileData struct {
	File huma.FormFile `form:"file" required:"true" doc:"CSV or XLSX file to upload (.csv, .xlsx)"`
}

type UploadFileInput struct {
	RawBody huma.MultipartFormFiles[UploadFileData]
}

type GetFileInput struct {
	ID string `path:"id" doc:"File UUID returned by POST /files"`
}

type DeleteFileInput struct {
	ID string `path:"id" doc:"File UUID returned by POST /files"`
}

type GetFileSchemaInput struct {
	ID string `path:"id" doc:"File UUID returned by POST /files"`
}

type GetFileSchemaOutput struct {
	Body struct {
		FileID   string               `json:"file_id"`
		Filename string               `json:"filename"`
		Tables   []s3client.TableMeta `json:"tables"`
	}
}

// ── Huma handlers ─────────────────────────────────────────────────────────────

func registerFileRoutes(api huma.API, deps FileDeps) {
	huma.Register(api, huma.Operation{
		OperationID:   "upload-file",
		Method:        http.MethodPost,
		Path:          "/files",
		Summary:       "Upload and index a CSV or XLSX file",
		Tags:          []string{"Files"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *UploadFileInput) (*GetFileOutput, error) {
		if deps.Embed == nil {
			return nil, huma.Error503ServiceUnavailable("embedding not configured")
		}
		if deps.S3 == nil {
			return nil, huma.Error503ServiceUnavailable("S3 not configured")
		}
		if deps.Qdrant == nil {
			return nil, huma.Error503ServiceUnavailable("Qdrant not configured")
		}

		start := time.Now()

		ff := input.RawBody.Data().File
		defer ff.Close()

		filename := ff.Filename
		fileID := uuid.New().String()

		rd, err := reader.New(filename)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}
		rawTables, err := rd.Extract(ctx, ff, filename)
		if err != nil {
			return nil, huma.Error500InternalServerError("extract: " + err.Error())
		}
		if len(rawTables) == 0 {
			return nil, huma.Error422UnprocessableEntity("no tables found in file")
		}

		// Generate table summaries on raw (pre-normalisation) data.
		summaries, sumErr := chunker.GenerateSummaryBatch(ctx, rawTables, deps.LLM, 4)
		if sumErr != nil {
			deps.Log.Warn("summary generation failed, continuing without summaries", "err", sumErr)
			summaries = make([]string, len(rawTables))
		}
		for i := range rawTables {
			rawTables[i].Summary = summaries[i]
		}

		normalized, err := ingest.ProcessAll(ctx, rawTables, deps.S3, 4)
		if err != nil {
			return nil, huma.Error500InternalServerError("process: " + err.Error())
		}

		s3Keys := make([]string, len(normalized))
		for i, t := range normalized {
			s3Keys[i] = t.S3Key
		}

		allChunks, err := chunker.Chunkify(ctx, normalized, deps.LLM, 4, fileID)
		if err != nil {
			return nil, huma.Error500InternalServerError("chunkify: " + err.Error())
		}
		var flat []chunker.Chunk
		for _, tc := range allChunks {
			flat = append(flat, tc...)
		}
		if len(flat) == 0 {
			return nil, huma.Error422UnprocessableEntity("all tables produced zero chunks")
		}

		vecs, err := embedder.EmbedAll(ctx, flat, deps.Embed, embedder.DefaultBatchSize)
		if err != nil {
			return nil, huma.Error500InternalServerError("embed: " + err.Error())
		}

		if err := deps.Qdrant.EnsureCollection(ctx, deps.EmbedDim); err != nil {
			return nil, huma.Error500InternalServerError("qdrant collection: " + err.Error())
		}
		for _, field := range []string{"s3_key", "file_id"} {
			if err := deps.Qdrant.EnsurePayloadIndex(ctx, field); err != nil {
				deps.Log.Warn("qdrant index failed", "field", field, "err", err)
			}
		}

		if err := deps.Qdrant.UpsertChunks(ctx, flat, vecs); err != nil {
			return nil, huma.Error500InternalServerError("qdrant upsert: " + err.Error())
		}

		tableMetas := make([]s3client.TableMeta, len(normalized))
		for i, nt := range normalized {
			cols := make([]s3client.ColumnSchema, len(nt.Columns))
			for j, c := range nt.Columns {
				cols[j] = s3client.ColumnSchema{
					Name:         c.Name,
					OriginalName: c.OriginalName,
					Type:         string(c.Type),
				}
			}
			tableMetas[i] = s3client.TableMeta{
				TableIndex: nt.TableIndex,
				SheetName:  nt.SheetName,
				S3Key:      nt.S3Key,
				RowCount:   nt.RowCount,
				Columns:    cols,
				Summary:    nt.Summary,
			}
		}

		rec := s3client.FileRecord{
			ID:          fileID,
			Filename:    filepath.Base(filename),
			UploadedAt:  time.Now().UTC(),
			TablesFound: len(normalized),
			ChunksCount: len(flat),
			S3Keys:      s3Keys,
			Tables:      tableMetas,
		}
		if err := deps.S3.SaveFileCatalog(ctx, rec); err != nil {
			deps.Log.Warn("catalog save failed", "file_id", fileID, "err", err)
		}

		m := metrics.Global
		m.IngestionFiles.Add(1)
		m.IngestionTables.Add(int64(len(normalized)))
		m.IngestionChunks.Add(int64(len(flat)))
		m.IngestionDurationMs.Add(time.Since(start).Milliseconds())

		deps.Log.Info("file indexed",
			"file_id", fileID,
			"filename", rec.Filename,
			"tables", rec.TablesFound,
			"chunks", rec.ChunksCount,
			"duration_ms", time.Since(start).Milliseconds(),
		)

		return &GetFileOutput{Body: rec}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-files",
		Method:      http.MethodGet,
		Path:        "/files",
		Summary:     "List all indexed files",
		Tags:        []string{"Files"},
	}, func(ctx context.Context, _ *struct{}) (*ListFilesOutput, error) {
		if deps.S3 == nil {
			return nil, huma.Error503ServiceUnavailable("S3 not configured")
		}
		records, err := deps.S3.ListFileCatalog(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError("list files: " + err.Error())
		}
		out := &ListFilesOutput{}
		out.Body.Files = records
		if out.Body.Files == nil {
			out.Body.Files = []s3client.FileRecord{}
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-file",
		Method:      http.MethodGet,
		Path:        "/files/{id}",
		Summary:     "Get metadata for a specific indexed file",
		Tags:        []string{"Files"},
	}, func(ctx context.Context, input *GetFileInput) (*GetFileOutput, error) {
		if deps.S3 == nil {
			return nil, huma.Error503ServiceUnavailable("S3 not configured")
		}
		rec, err := deps.S3.GetFileCatalog(ctx, input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("file not found")
		}
		return &GetFileOutput{Body: *rec}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-file-schema",
		Method:      http.MethodGet,
		Path:        "/files/{id}/schema",
		Summary:     "Get column schema for all tables in a file",
		Tags:        []string{"Files"},
	}, func(ctx context.Context, input *GetFileSchemaInput) (*GetFileSchemaOutput, error) {
		if deps.S3 == nil {
			return nil, huma.Error503ServiceUnavailable("S3 not configured")
		}
		rec, err := deps.S3.GetFileCatalog(ctx, input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("file not found")
		}
		out := &GetFileSchemaOutput{}
		out.Body.FileID = rec.ID
		out.Body.Filename = rec.Filename
		out.Body.Tables = rec.Tables
		if out.Body.Tables == nil {
			out.Body.Tables = []s3client.TableMeta{}
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "delete-file",
		Method:        http.MethodDelete,
		Path:          "/files/{id}",
		Summary:       "Delete a file and all its indexed data from Qdrant and S3",
		Tags:          []string{"Files"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *DeleteFileInput) (*struct{}, error) {
		if deps.S3 == nil || deps.Qdrant == nil {
			return nil, huma.Error503ServiceUnavailable("S3 or Qdrant not configured")
		}

		rec, err := deps.S3.GetFileCatalog(ctx, input.ID)
		if err != nil {
			return nil, huma.Error404NotFound("file not found")
		}

		var errs []string

		if err := deps.Qdrant.DeleteByFileID(ctx, input.ID); err != nil {
			errs = append(errs, "qdrant: "+err.Error())
		}
		if err := deps.S3.DeleteParquetFiles(ctx, rec.S3Keys); err != nil {
			errs = append(errs, "s3 parquet: "+err.Error())
		}
		// Remove catalog entry last — so retries can re-read S3Keys if earlier steps failed.
		if err := deps.S3.DeleteFileCatalog(ctx, input.ID); err != nil {
			errs = append(errs, "s3 catalog: "+err.Error())
		}

		if len(errs) > 0 {
			deps.Log.Warn("partial delete failure", "file_id", input.ID, "errors", errs)
		}

		return nil, nil
	})
}
