package provider

import "context"

// EmbeddingProvider produces dense vector embeddings for a batch of texts.
// Implementations: VoyageAIClient, OpenAIEmbedClient.
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
