package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ETretyakov/rag-with-tables/internal/config"
)

const (
	anthropicBaseURL = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
	anthropicMaxTok  = 2048
	anthropicTimeout = 60 * time.Second
)

// AnthropicClient calls the Anthropic Messages API.
type AnthropicClient struct {
	cfg  config.AnthropicConfig
	http *http.Client
}

func newAnthropicClient(cfg config.AnthropicConfig) *AnthropicClient {
	return &AnthropicClient{cfg: cfg, http: &http.Client{Timeout: anthropicTimeout}}
}

func (c *AnthropicClient) Complete(ctx context.Context, messages []Message) (string, error) {
	system, msgs := extractSystem(messages)

	body := map[string]any{
		"model":      c.cfg.Model,
		"max_tokens": anthropicMaxTok,
		"messages":   toAnthropicMsgs(msgs),
	}
	if system != "" {
		body["system"] = system
	}

	raw, err := c.doPost(ctx, "/v1/messages", body)
	if err != nil {
		return "", err
	}

	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("anthropic: parse response: %w", err)
	}
	if resp.Error != nil {
		return "", fmt.Errorf("anthropic %s: %s", resp.Error.Type, resp.Error.Message)
	}
	for _, b := range resp.Content {
		if b.Type == "text" {
			return b.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: no text content in response")
}

// StreamComplete implements StreamLLMProvider using Anthropic's streaming Messages API.
// Each text_delta event calls yield with the incremental token.
func (c *AnthropicClient) StreamComplete(ctx context.Context, messages []Message, yield func(token string)) error {
	system, msgs := extractSystem(messages)

	body := map[string]any{
		"model":      c.cfg.Model,
		"max_tokens": anthropicMaxTok,
		"stream":     true,
		"messages":   toAnthropicMsgs(msgs),
	}
	if system != "" {
		body["system"] = system
	}

	data, _ := json.Marshal(body)
	// Use a no-timeout client for streaming — context cancellation handles it.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		anthropicBaseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("anthropic stream: build request: %w", err)
	}
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	streamClient := &http.Client{} // no timeout — rely on context
	resp, err := streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("anthropic stream: HTTP %d: %s", resp.StatusCode, raw)
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
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if event.Type == "content_block_delta" && event.Delta.Type == "text_delta" {
			yield(event.Delta.Text)
		}
	}
	return scanner.Err()
}

// extractSystem pulls system messages out; Anthropic uses a top-level field for them.
func extractSystem(msgs []Message) (system string, rest []Message) {
	for _, m := range msgs {
		if m.Role == RoleSystem {
			system = m.Content
		} else {
			rest = append(rest, m)
		}
	}
	return
}

func toAnthropicMsgs(msgs []Message) []map[string]string {
	out := make([]map[string]string, len(msgs))
	for i, m := range msgs {
		out[i] = map[string]string{"role": m.Role, "content": m.Content}
	}
	return out
}

func (c *AnthropicClient) doPost(ctx context.Context, path string, body any) ([]byte, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, raw)
	}
	return raw, nil
}
