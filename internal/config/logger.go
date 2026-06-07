package config

type LoggerConfig struct {
	Level  string `env:"LEVEL"  envDefault:"info"`
	Format string `env:"FORMAT" envDefault:"json"`
}
