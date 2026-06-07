package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ETretyakov/rag-with-tables/internal/query"
)

// streamHandler handles POST /query/stream via Server-Sent Events.
// It cannot go through Huma because Huma v2 buffers the full response body.
//
// Event format:
//
//	data: {"type":"token","content":"..."}\n\n   — one per LLM token
//	data: {"type":"result","answer":"...","sources":[...],"sql_used":"..."}\n\n
//	data: {"type":"error","message":"..."}\n\n    — on failure
func streamHandler(deps QueryDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Pipeline == nil {
			http.Error(w, `{"error":"query pipeline not configured"}`, http.StatusServiceUnavailable)
			return
		}

		// Parse request body.
		var req struct {
			Query string `json:"query"`
			TopK  int    `json:"top_k"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}
		if req.Query == "" {
			http.Error(w, `{"error":"query is required"}`, http.StatusBadRequest)
			return
		}
		if req.TopK <= 0 {
			req.TopK = 10
		}

		// Set SSE headers before writing the first byte.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, canFlush := w.(http.Flusher)
		sendEvent := func(payload any) {
			data, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if canFlush {
				flusher.Flush()
			}
		}

		yield := func(token string) {
			sendEvent(map[string]string{"type": "token", "content": token})
		}

		result, err := deps.Pipeline.StreamQuery(r.Context(), query.QueryRequest{
			Query: req.Query,
			TopK:  req.TopK,
		}, yield)
		if err != nil {
			sendEvent(map[string]string{"type": "error", "message": err.Error()})
			return
		}

		sendEvent(map[string]any{
			"type":         "result",
			"answer":       result.Answer,
			"sources":      result.Sources,
			"sql_attempts": result.SQLAttempts,
		})
	}
}
