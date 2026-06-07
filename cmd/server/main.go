package main

import (
	"log/slog"
	"os"

	"github.com/ETretyakov/rag-with-tables/internal/app"
	"github.com/ETretyakov/rag-with-tables/internal/config"
	"github.com/ETretyakov/rag-with-tables/internal/logger"
)

func main() {
	cfg := config.MustLoad()
	log := logger.New(cfg.Logger)
	slog.SetDefault(log)

	a, err := app.New(cfg, log)
	if err != nil {
		log.Error("init failed", "error", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		log.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
