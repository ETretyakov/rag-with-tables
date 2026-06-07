package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ETretyakov/rag-with-tables/internal/config"
)

const openaiTimeout = 120 * time.Second

// OpenAICompatClient calls /chat/completions on any OpenAI-compatible endpoint.
// Works with OpenAI, Ollama, vLLM, LM Studio, and similar servers.
type OpenAICompatClient struct {
	cfg  config.OpenAICompatConfig
	http *http.Client
}

func newOpenAICompatClient(cfg config.OpenAICompatConfig) *OpenAICompatClient {
	return &OpenAICompatClient{cfg: cfg, http: &http.Client{Timeout: openaiTimeout}}
}

func (c *OpenAICompatClient) Complete(ctx context.Context, messages []Message) (string, error) {
	msgs := make([]map[string]string, len(messages))
	for i, m := range messages {
		msgs[i] = map[string]string{"role": m.Role, "content": m.Content}
	}

	body := map[string]any{
		"model":    c.cfg.Model,
		"messages": msgs,
	}

	raw, err := c.doPost(ctx, "/chat/completions", body)
	if err != nil {
		return "", err
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("openai: parse response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("openai: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices in response")
	}
	return resp.Choices[0].Message.Content, nil
}

// StreamComplete implements StreamLLMProvider using the OpenAI-compatible streaming API.
func (c *OpenAICompatClient) StreamComplete(ctx context.Context, messages []Message, yield func(token string)) error {
	msgs := make([]map[string]string, len(messages))
	for i, m := range messages {
		msgs[i] = map[string]string{"role": m.Role, "content": m.Content}
	}

	body := map[string]any{
		"model":    c.cfg.Model,
		"messages": msgs,
		"stream":   true,
	}

	data, _ := json.Marshal(body)
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("openai stream: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	streamClient := &http.Client{} // no timeout — rely on context
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("openai stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai stream: HTTP %d: %s", resp.StatusCode, raw)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := line[len("data: "):]
		if payload == "[DONE]" {
			break
		}

		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if len(event.Choices) > 0 {
			if tok := event.Choices[0].Delta.Content; tok != "" {
				yield(tok)
			}
		}
	}
	return scanner.Err()
}

func (c *OpenAICompatClient) doPost(ctx context.Context, path string, body any) ([]byte, error) {
	data, _ := json.Marshal(body)
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, raw)
	}
	return raw, nil
}

// ── OpenAI-compatible embedding client ────────────────────────────────────────

// OpenAIEmbedClient calls /embeddings on any OpenAI-compatible endpoint
// (Ollama, vLLM, LM Studio, etc.).
type OpenAIEmbedClient struct {
	cfg  config.OpenAIEmbedConfig
	http *http.Client
}

func NewOpenAIEmbedClient(cfg config.OpenAIEmbedConfig) *OpenAIEmbedClient {
	return &OpenAIEmbedClient{cfg: cfg, http: &http.Client{Timeout: openaiTimeout}}
}

func (c *OpenAIEmbedClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body := map[string]any{
		"model": c.cfg.Model,
		"input": texts,
	}

	data, _ := json.Marshal(body)
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai-embed: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai-embed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai-embed: HTTP %d: %s", resp.StatusCode, raw)
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
		return nil, fmt.Errorf("openai-embed: parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("openai-embed: %s", result.Error.Message)
	}

	sort.Slice(result.Data, func(i, j int) bool {
		return result.Data[i].Index < result.Data[j].Index
	})

	out := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		out[i] = d.Embedding
	}
	return out, nil
}
