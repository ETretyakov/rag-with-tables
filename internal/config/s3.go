package config

type S3Config struct {
	Endpoint     string `env:"ENDPOINT"       envDefault:"http://localhost:9000"`
	Bucket       string `env:"BUCKET"         envDefault:"table-rags"`
	AccessKey    string `env:"ACCESS_KEY"     envDefault:"minioadmin"`
	SecretKey    string `env:"SECRET_KEY"     envDefault:"minioadmin"`
	Region       string `env:"REGION"         envDefault:"us-east-1"`
	UsePathStyle bool   `env:"USE_PATH_STYLE" envDefault:"true"`
}
