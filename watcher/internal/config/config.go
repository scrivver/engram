package config

import (
	"fmt"
	"os"
	"strings"
)

var defaultIgnore = []string{
	".git",
	".DS_Store",
	"node_modules",
	"__pycache__",
	".venv",
	".data",
	"tmp",
}

type Config struct {
	WatchDirs      []string
	IgnorePatterns []string
	DeviceName     string
	RabbitMQPort   string
}

func Load() (*Config, error) {
	dirs := os.Getenv("WATCH_DIRS")
	if dirs == "" {
		return nil, fmt.Errorf("WATCH_DIRS is required (comma-separated list of directories)")
	}

	deviceName := os.Getenv("DEVICE_NAME")
	if deviceName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("DEVICE_NAME not set and cannot get hostname: %w", err)
		}
		deviceName = hostname
	}

	ignore := defaultIgnore
	if extra := os.Getenv("WATCH_IGNORE"); extra != "" {
		ignore = append(ignore, strings.Split(extra, ",")...)
	}

	return &Config{
		WatchDirs:      strings.Split(dirs, ","),
		IgnorePatterns: ignore,
		DeviceName:     deviceName,
		RabbitMQPort:   envOr("RABBITMQ_AMQP_PORT", "5672"),
	}, nil
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
