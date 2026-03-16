package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port   string
	PGHost string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:   envOr("PORT", "8080"),
		PGHost: os.Getenv("PGHOST"),
	}

	if cfg.PGHost == "" {
		return nil, fmt.Errorf("PGHOST is required")
	}

	return cfg, nil
}

func (c *Config) PGDSN() string {
	return fmt.Sprintf("host=%s dbname=engram sslmode=disable", c.PGHost)
}

func (c *Config) PGMigrateURL() string {
	return fmt.Sprintf("pgx5:///?host=%s&dbname=engram&sslmode=disable", c.PGHost)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
