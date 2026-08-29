// Package config loads application configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings for the proxy server.
type Config struct {
	YouTrackURL    string
	Port           string
	MaxConcurrency int           // YT_MAX_CONCURRENCY, default 10, range [1, 100]
	QueueTimeout   time.Duration // YT_QUEUE_TIMEOUT_SECONDS, default 30s, range [1s, 300s]
	RequestTimeout time.Duration // YT_REQUEST_TIMEOUT_SECONDS, default 30s, range [1s, 300s]
	AuthUsername   string        // AUTH_USERNAME, default "", trimmed of whitespace
}

// LoadConfig reads environment variables and returns a populated Config.
func LoadConfig() (*Config, error) {
	return &Config{
		YouTrackURL:    envStr("YOUTRACK_URL", "https://example.youtrack.cloud"),
		Port:           envStr("PORT", "8080"),
		MaxConcurrency: envIntClamped("YT_MAX_CONCURRENCY", 10, 1, 100),
		QueueTimeout:   time.Duration(envIntClamped("YT_QUEUE_TIMEOUT_SECONDS", 30, 1, 300)) * time.Second,
		RequestTimeout: time.Duration(envIntClamped("YT_REQUEST_TIMEOUT_SECONDS", 30, 1, 300)) * time.Second,
		AuthUsername:   strings.TrimSpace(os.Getenv("AUTH_USERNAME")),
	}, nil
}

// envStr returns the environment variable value or a default.
func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envIntClamped parses an integer env var, clamping it to [lo, hi].
func envIntClamped(key string, fallback, lo, hi int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
