package provider

import (
	"fmt"
	"strings"

	"github.com/ETretyakov/rag-with-tables/internal/config"
)

// NewLLMProvider creates the LLMProvider specified by cfg.Provider.
// Supported values: "anthropic", "openai".
func NewLLMProvider(cfg config.LLMConfig) (LLMProvider, error) {
	switch strings.ToLower(cfg.Provider) {
	case "anthropic":
		if cfg.Anthropic.APIKey == "" {
			return nil, fmt.Errorf("LLM_ANTHROPIC_API_KEY is required for provider=anthropic")
		}
		return newAnthropicClient(cfg.Anthropic), nil
	case "openai":
		return newOpenAICompatClient(cfg.OpenAI), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q (supported: anthropic, openai)", cfg.Provider)
	}
}

// NewEmbeddingProvider creates the EmbeddingProvider specified by cfg.Provider.
// Supported values: "voyageai", "openai".
func NewEmbeddingProvider(cfg config.EmbeddingConfig) (EmbeddingProvider, error) {
	switch strings.ToLower(cfg.Provider) {
	case "voyageai":
		if cfg.VoyageAI.APIKey == "" {
			return nil, fmt.Errorf("EMBEDDING_VOYAGEAI_API_KEY is required for provider=voyageai")
		}
		return NewVoyageAIClient(cfg.VoyageAI), nil
	case "openai":
		return NewOpenAIEmbedClient(cfg.OpenAI), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider %q (supported: voyageai, openai)", cfg.Provider)
	}
}
