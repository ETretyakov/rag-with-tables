package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ETretyakov/rag-with-tables/internal/config"
	"github.com/ETretyakov/rag-with-tables/internal/metrics"
)

type HealthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"Service status"`
	}
}

// Deps bundles all route-group dependencies for the API server.
type Deps struct {
	Files FileDeps
	Query QueryDeps
}

// NewServer creates the main API HTTP server with all routes registered.
func NewServer(cfg config.ServerConfig, log *slog.Logger, deps Deps) *http.Server {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(log))

	humaConfig := huma.DefaultConfig("Table RAG API", "0.1.0")
	humaConfig.Info.Description = "RAG-система для табличных данных (CSV/XLSX)"

	humaAPI := humachi.New(r, humaConfig)

	// System routes (Huma — full Swagger support)
	huma.Register(humaAPI, huma.Operation{
		OperationID: "health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check",
		Tags:        []string{"System"},
	}, func(ctx context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	registerFileRoutes(humaAPI, deps.Files)

	// POST /query (buffered)
	registerQueryRoutes(humaAPI, deps.Query)

	// POST /query/stream — SSE, must use chi directly (Huma buffers response)
	r.Post("/query/stream", streamHandler(deps.Query))

	// GET /metrics
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metrics.Global.Snapshot())
	})

	// Serve the frontend SPA — must be registered last so API routes take priority.
	r.Handle("/*", webHandler())

	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
			)
		})
	}
}
