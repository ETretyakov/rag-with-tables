package provider

import "context"

// Role constants for LLM conversation turns.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is a single turn in an LLM conversation.
type Message struct {
	Role    string
	Content string
}

// LLMProvider is the single interface for all text-generation backends.
type LLMProvider interface {
	Complete(ctx context.Context, messages []Message) (string, error)
}

// StreamLLMProvider extends LLMProvider with token-by-token streaming.
// yield is called once per token chunk as the model generates text.
// If a provider does not implement this interface, callers should fall back
// to Complete and emit the full response at once.
type StreamLLMProvider interface {
	LLMProvider
	StreamComplete(ctx context.Context, messages []Message, yield func(token string)) error
}
