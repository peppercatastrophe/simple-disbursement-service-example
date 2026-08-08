package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration, loaded from environment variables.
type Config struct {
	Port           string
	DatabaseURL    string
	JWTSecret      string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	IdempotencyTTL time.Duration
}

// Load reads configuration from the environment with defaults.
func Load() *Config {
	return &Config{
		Port:           getEnv("APP_PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", ""),
		JWTSecret:      getEnv("JWT_SECRET", "dev-secret-change-me"),
		AccessTTL:      getDur("ACCESS_TTL", 15*time.Minute),
		RefreshTTL:     getDur("REFRESH_TTL", 7*24*time.Hour),
		IdempotencyTTL: getDur("IDEMPOTENCY_TTL", 24*time.Hour),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getDur(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
