package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port            string
	PGHost          string
	RabbitMQPort    string
	StorageBackend  string
	StorageFSRoot   string
	StorageS3Endpoint  string
	StorageS3AccessKey string
	StorageS3SecretKey string
	StorageS3Bucket    string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:            envOr("PORT", "8080"),
		PGHost:          os.Getenv("PGHOST"),
		RabbitMQPort:    envOr("RABBITMQ_AMQP_PORT", "5672"),
		StorageBackend:  envOr("STORAGE_BACKEND", "fs"),
		StorageFSRoot:   envOr("STORAGE_FS_ROOT", ".data/storage"),
		StorageS3Endpoint:  os.Getenv("STORAGE_S3_ENDPOINT"),
		StorageS3AccessKey: os.Getenv("STORAGE_S3_ACCESS_KEY"),
		StorageS3SecretKey: os.Getenv("STORAGE_S3_SECRET_KEY"),
		StorageS3Bucket:    envOr("STORAGE_S3_BUCKET", "engram"),
	}

	if cfg.PGHost == "" {
		return nil, fmt.Errorf("PGHOST is required")
	}

	if cfg.StorageBackend != "fs" && cfg.StorageBackend != "s3" {
		return nil, fmt.Errorf("STORAGE_BACKEND must be 'fs' or 's3', got %q", cfg.StorageBackend)
	}

	if cfg.StorageBackend == "s3" {
		if cfg.StorageS3Endpoint == "" || cfg.StorageS3AccessKey == "" || cfg.StorageS3SecretKey == "" {
			return nil, fmt.Errorf("S3 storage requires STORAGE_S3_ENDPOINT, STORAGE_S3_ACCESS_KEY, STORAGE_S3_SECRET_KEY")
		}
	}

	return cfg, nil
}

// PGDSN returns a key=value connection string for pgxpool.
func (c *Config) PGDSN() string {
	return fmt.Sprintf("host=%s dbname=engram sslmode=disable", c.PGHost)
}

// PGMigrateURL returns a pgx5:// URL for golang-migrate (unix socket).
func (c *Config) PGMigrateURL() string {
	return fmt.Sprintf("pgx5:///?host=%s&dbname=engram&sslmode=disable", c.PGHost)
}

func (c *Config) RabbitMQURL() string {
	return fmt.Sprintf("amqp://guest:guest@127.0.0.1:%s/", c.RabbitMQPort)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
