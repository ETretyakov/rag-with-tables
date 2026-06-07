package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/ETretyakov/rag-with-tables/internal/config"
)

const (
	voyageBaseURL = "https://api.voyageai.com"
	voyageTimeout = 60 * time.Second
)

// VoyageAIClient calls the VoyageAI embeddings API.
type VoyageAIClient struct {
	cfg  config.VoyageAIConfig
	http *http.Client
}

func NewVoyageAIClient(cfg config.VoyageAIConfig) *VoyageAIClient {
	return &VoyageAIClient{cfg: cfg, http: &http.Client{Timeout: voyageTimeout}}
}

func (c *VoyageAIClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body := map[string]any{
		"model":      c.cfg.Model,
		"input":      texts,
		"input_type": "document", // "query" is used at search time
	}

	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		voyageBaseURL+"/v1/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("voyageai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyageai: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("voyageai: HTTP %d: %s", resp.StatusCode, raw)
	}

	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("voyageai: parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("voyageai: %s", result.Error.Message)
	}

	// API may return results out of order.
	sort.Slice(result.Data, func(i, j int) bool {
		return result.Data[i].Index < result.Data[j].Index
	})

	out := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		out[i] = d.Embedding
	}
	return out, nil
}
