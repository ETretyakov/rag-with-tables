package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	Server    ServerConfig    `envPrefix:"SERVER_"`
	Logger    LoggerConfig    `envPrefix:"LOG_"`
	LLM       LLMConfig       `envPrefix:"LLM_"`
	Embedding EmbeddingConfig `envPrefix:"EMBEDDING_"`
	Qdrant    QdrantConfig    `envPrefix:"QDRANT_"`
	S3        S3Config        `envPrefix:"S3_"`
	DuckDB    DuckDBConfig    `envPrefix:"DUCKDB_"`
}

// Load читает .env (если есть) и заполняет Config из переменных окружения.
func Load() (*Config, error) {
	_ = godotenv.Load() // игнорируем ошибку — .env необязателен

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}
