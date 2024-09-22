package config

import (
	"fmt"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

var EnvPreffix = "TAG"

type Config struct {
	Host string `default:"0.0.0.0"`
	Port string `default:"4001"`
}

func New() *Config {
	godotenv.Load()
	cfg, err := Get()
	if err != nil {
		panic(fmt.Errorf("invalid value(s) retrieved from environment: %w", err))
	}
	return cfg
}

func Get() (*Config, error) {
	cfg := Config{}
	err := envconfig.Process(EnvPreffix, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
