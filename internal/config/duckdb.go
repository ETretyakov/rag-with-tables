package config

import "time"

type DuckDBConfig struct {
	CacheTTL       time.Duration `env:"CACHE_TTL"        envDefault:"15m"`
	CacheMaxTables int           `env:"CACHE_MAX_TABLES" envDefault:"100"`
}
