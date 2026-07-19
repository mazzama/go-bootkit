package main

import (
	"fmt"
	"os"
)

// Config holds everything the service needs to boot. Values come from the
// environment, with sane defaults for local development. DB_CONN_STR is the
// only required setting — the service fails fast if it is missing.
type Config struct {
	// DBConnStr is the Postgres connection string (required).
	DBConnStr string
	// RedisAddr is the Redis host:port. Defaults to localhost:6379.
	RedisAddr string
	// RedisPassword is the Redis password, if any. Defaults to empty.
	RedisPassword string
	// HTTPAddr is the address the HTTP server listens on. Defaults to :8080.
	HTTPAddr string
}

// LoadConfig reads configuration from the environment and applies defaults.
// It returns an error if a required value is missing.
func LoadConfig() (Config, error) {
	cfg := Config{
		DBConnStr:     os.Getenv("DB_CONN_STR"),
		RedisAddr:     envOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		HTTPAddr:      envOrDefault("HTTP_ADDR", ":8080"),
	}

	if cfg.DBConnStr == "" {
		return Config{}, fmt.Errorf("DB_CONN_STR is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
