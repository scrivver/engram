package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port   string
	PGHost string

	AuthMode          string
	OIDCIssuerURL     string
	OIDCUsernameClaim string

	// PresignURLTemplate is used to render download_url for s3-backed files.
	// Must include "{file_path}" which is replaced with the url-encoded key.
	PresignURLTemplate string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:               envOr("PORT", "8080"),
		PGHost:             os.Getenv("PGHOST"),
		AuthMode:           envOr("AUTH_MODE", ""),
		OIDCIssuerURL:      os.Getenv("OIDC_ISSUER_URL"),
		OIDCUsernameClaim:  envOr("OIDC_USERNAME_CLAIM", "preferred_username"),
		PresignURLTemplate: os.Getenv("PRESIGN_URL_TEMPLATE"),
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
