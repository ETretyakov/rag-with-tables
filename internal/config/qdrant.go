package config

type QdrantConfig struct {
	Host       string `env:"HOST"        envDefault:"localhost"`
	Port       int    `env:"PORT"        envDefault:"6333"`
	GRPCPort   int    `env:"GRPC_PORT"   envDefault:"6334"`
	APIKey     string `env:"API_KEY"`
	UseTLS     bool   `env:"USE_TLS"     envDefault:"false"`
	Collection string `env:"COLLECTION"  envDefault:"table-rags"`
}
