package probes

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ETretyakov/rag-with-tables/internal/config"
)

// NewServer создаёт отдельный HTTP-сервер для Kubernetes probes.
// Вынесен на отдельный порт, чтобы продолжать отвечать во время graceful shutdown основного сервера.
func NewServer(cfg config.ServerConfig) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /live", handleLive)
	mux.HandleFunc("GET /ready", handleReady)

	return &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.ProbesPort),
		Handler: mux,
	}
}

func handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

func handleReady(w http.ResponseWriter, _ *http.Request) {
	// в будущих фазах сюда добавятся проверки Qdrant и S3
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
