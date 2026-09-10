// Package config loads runtime configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration.
type Config struct {
	Addr         string
	DatabaseURL  string
	RedisAddr    string
	RedisPassword string
	JWTSecret    string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	AIAPIKey     string
	MapAPIKey    string
	CORSOrigins  []string
	MigrateDir   string
	RunMigrate   bool
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load reads configuration from environment variables with sensible dev defaults.
func Load() *Config {
	c := &Config{
		Addr:          getenv("ADDR", ":8080"),
		DatabaseURL:   getenv("DATABASE_URL", "postgres://tripweave:tripweave@127.0.0.1:5433/tripweave?sslmode=disable"),
		RedisAddr:     getenv("REDIS_ADDR", "127.0.0.1:6380"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		JWTSecret:     getenv("JWT_SECRET", "change-me-in-production"),
		AccessTTL:     parseDuration("ACCESS_TOKEN_TTL", 30*time.Minute),
		RefreshTTL:    parseDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		AIAPIKey:      os.Getenv("AI_API_KEY"),
		MapAPIKey:     os.Getenv("MAP_API_KEY"),
		MigrateDir:    getenv("MIGRATE_DIR", "file://migrations"),
	}
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		c.CORSOrigins = splitComma(v)
	} else {
		c.CORSOrigins = []string{"http://localhost:5173"}
	}
	c.RunMigrate = true
	if v := os.Getenv("RUN_MIGRATE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.RunMigrate = b
		}
	}
	return c
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}




