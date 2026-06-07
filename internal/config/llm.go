package config

type LLMConfig struct {
	Provider  string            `env:"PROVIDER" envDefault:"anthropic"`
	Anthropic AnthropicConfig   `envPrefix:"ANTHROPIC_"`
	OpenAI    OpenAICompatConfig `envPrefix:"OPENAI_"`
}

type AnthropicConfig struct {
	APIKey string `env:"API_KEY"`
	Model  string `env:"MODEL" envDefault:"claude-sonnet-4-6"`
}

// OpenAICompatConfig используется для Anthropic-совместимого API (Ollama, vLLM и др.)
type OpenAICompatConfig struct {
	BaseURL string `env:"BASE_URL" envDefault:"http://localhost:11434/v1"`
	APIKey  string `env:"API_KEY"  envDefault:"ollama"`
	Model   string `env:"MODEL"    envDefault:"llama3.2"`
}
