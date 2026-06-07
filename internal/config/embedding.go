package config

type EmbeddingConfig struct {
	Provider string              `env:"PROVIDER" envDefault:"voyageai"`
	Dims     int                 `env:"DIMS"     envDefault:"1024"` // vector size; voyage-3=1024, nomic-embed-text=768
	VoyageAI VoyageAIConfig      `envPrefix:"VOYAGEAI_"`
	OpenAI   OpenAIEmbedConfig   `envPrefix:"OPENAI_"`
}

type VoyageAIConfig struct {
	APIKey string `env:"API_KEY"`
	Model  string `env:"MODEL" envDefault:"voyage-3"`
}

type OpenAIEmbedConfig struct {
	BaseURL string `env:"BASE_URL" envDefault:"http://localhost:11434/v1"`
	APIKey  string `env:"API_KEY"  envDefault:"ollama"`
	Model   string `env:"MODEL"    envDefault:"nomic-embed-text"`
}
