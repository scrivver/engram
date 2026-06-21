package config

import (
	"fmt"
	"net/url"
	"os"
)

type Config struct {
	Port       string
	PGHost     string
	PGUser     string
	PGPassword string
	PGDatabase string

	AuthMode          string
	OIDCIssuerURL     string
	OIDCClientID      string
	OIDCUsernameClaim string
	OIDCRedirectURI   string

	// PresignURLTemplate is used to render download_url for s3-backed files.
	// Must include "{file_path}" which is replaced with the url-encoded key.
	PresignURLTemplate string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:               envOr("PORT", "8080"),
		PGHost:             os.Getenv("PGHOST"),
		PGUser:             envOr("PGUSER", "postgres"),
		PGPassword:         os.Getenv("PGPASSWORD"),
		PGDatabase:         envOr("PGDATABASE", "engram"),
		AuthMode:           envOr("AUTH_MODE", ""),
		OIDCIssuerURL:      os.Getenv("OIDC_ISSUER_URL"),
		OIDCClientID:       envOr("OIDC_CLIENT_ID", "mind-palace"),
		OIDCUsernameClaim:  envOr("OIDC_USERNAME_CLAIM", "preferred_username"),
		OIDCRedirectURI:    envOr("OIDC_REDIRECT_URI", "com.mindpalace.app://callback"),
		PresignURLTemplate: os.Getenv("PRESIGN_URL_TEMPLATE"),
	}

	if cfg.PGHost == "" {
		return nil, fmt.Errorf("PGHOST is required")
	}

	return cfg, nil
}

func (c *Config) PGDSN() string {
	dsn := fmt.Sprintf("host=%s user=%s dbname=%s sslmode=disable", c.PGHost, c.PGUser, c.PGDatabase)
	if c.PGPassword != "" {
		dsn += fmt.Sprintf(" password=%s", c.PGPassword)
	}
	return dsn
}

func (c *Config) PGMigrateURL() string {
	password := ""
	if c.PGPassword != "" {
		password = "&password=" + url.QueryEscape(c.PGPassword)
	}
	return fmt.Sprintf("pgx5:///?host=%s&user=%s&dbname=%s&sslmode=disable%s", url.QueryEscape(c.PGHost), url.QueryEscape(c.PGUser), url.QueryEscape(c.PGDatabase), password)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
