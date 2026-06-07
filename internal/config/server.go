package config

import "time"

type ServerConfig struct {
	Host            string        `env:"HOST"             envDefault:"0.0.0.0"`
	Port            int           `env:"PORT"             envDefault:"8080"`
	ProbesPort      int           `env:"PROBES_PORT"      envDefault:"8081"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT"     envDefault:"30s"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT"    envDefault:"30s"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`
}
