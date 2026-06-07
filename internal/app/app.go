package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ETretyakov/rag-with-tables/internal/api"
	"github.com/ETretyakov/rag-with-tables/internal/config"
	"github.com/ETretyakov/rag-with-tables/internal/probes"
	"github.com/ETretyakov/rag-with-tables/internal/provider"
	"github.com/ETretyakov/rag-with-tables/internal/query"
	duckdbcache "github.com/ETretyakov/rag-with-tables/internal/storage/duckdb"
	s3client "github.com/ETretyakov/rag-with-tables/internal/storage/s3"
	qdrantclient "github.com/ETretyakov/rag-with-tables/internal/vectordb/qdrant"
)

type App struct {
	cfg    *config.Config
	log    *slog.Logger
	api    *http.Server
	probes *http.Server
}

func New(cfg *config.Config, log *slog.Logger) (*App, error) {
	// ── Optional services — log warning instead of failing if not configured ──

	var s3c *s3client.Client
	if s3, err := s3client.New(cfg.S3); err != nil {
		log.Warn("S3 client init failed — ingestion disabled", "err", err)
	} else {
		s3c = s3
	}

	qdrant := qdrantclient.New(cfg.Qdrant)

	var llm provider.LLMProvider
	if lp, err := provider.NewLLMProvider(cfg.LLM); err != nil {
		log.Warn("LLM provider init failed — HYDE disabled", "err", err)
	} else {
		llm = lp
	}

	var embed provider.EmbeddingProvider
	if ep, err := provider.NewEmbeddingProvider(cfg.Embedding); err != nil {
		log.Warn("embedding provider init failed — ingestion disabled", "err", err)
	} else {
		embed = ep
	}

	// DuckDB table cache — only useful when S3 is available.
	var dbCache *duckdbcache.TableCache
	if s3c != nil {
		dbCache = duckdbcache.NewTableCache(s3c, cfg.DuckDB)
	}

	deps := api.Deps{
		Files: api.FileDeps{
			S3:       s3c,
			Qdrant:   qdrant,
			LLM:      llm,
			Embed:    embed,
			EmbedDim: cfg.Embedding.Dims,
			Log:      log,
		},
		Query: api.QueryDeps{
			Pipeline: query.New(qdrant, embed, llm, dbCache, log),
		},
	}

	return &App{
		cfg:    cfg,
		log:    log,
		api:    api.NewServer(cfg.Server, log, deps),
		probes: probes.NewServer(cfg.Server),
	}, nil
}

func (a *App) Run() error {
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// baseCtx is the parent of every request context. Cancelling it causes all
	// active handlers (including SSE streams) to see ctx.Done() and exit cleanly,
	// so http.Server.Shutdown() doesn't have to wait for them to finish on their own.
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	a.api.BaseContext = func(_ net.Listener) context.Context { return baseCtx }

	errCh := make(chan error, 2)

	go func() {
		a.log.Info("starting API server", "addr", a.api.Addr)
		if err := a.api.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("api server: %w", err)
		}
	}()

	go func() {
		a.log.Info("starting probes server", "addr", a.probes.Addr)
		if err := a.probes.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("probes server: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		// One server failed to start — shut down the other before returning.
		a.log.Error("server error, initiating shutdown", "err", err)
		cancelBase()
		_ = a.shutdown()
		return err
	case <-sigCtx.Done():
		a.log.Info("shutdown signal received")
		cancelBase() // unblock all active SSE / streaming handlers immediately
		return a.shutdown()
	}
}

func (a *App) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.Server.ShutdownTimeout)
	defer cancel()

	var errs []error
	if err := a.api.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("api shutdown: %w", err))
	}
	if err := a.probes.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("probes shutdown: %w", err))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	a.log.Info("shutdown complete")
	return nil
}
