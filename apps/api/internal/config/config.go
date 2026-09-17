// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// defaultJWTSecret is the dev-only fallback; production must override it.
const defaultJWTSecret = "change-me-in-production"

// ErrInsecureJWTSecret is returned when production runs with the dev JWT secret.
var ErrInsecureJWTSecret = errors.New("JWT_SECRET must be set to a strong secret when APP_ENV=production")

// Config holds all runtime configuration.
type Config struct {
	Addr            string
	Env             string // "development" (default) or "production"
	DatabaseURL     string
	RedisAddr       string
	RedisPassword   string
	RedisTLS        bool // 托管 Redis（Upstash 等）需要 TLS
	JWTSecret       string
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	AIAPIKey        string
	AIProvider      string
	AIBaseURL       string
	AIModel         string
	AITimeout       time.Duration
	AIMaxToolRounds int
	MapAPIKey       string
	CORSOrigins     []string
	MigrateDir      string
	RunMigrate      bool
	SMTPHost        string
	SMTPPort        string
	SMTPUser        string
	SMTPPass        string
	SMTPFrom        string
	AppBaseURL      string
}

// IsProd reports whether the process runs in production mode.
func (c *Config) IsProd() bool { return c.Env == "production" }

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load reads configuration from environment variables with sensible dev defaults.
// Malformed values log a warning and fall back instead of being silently ignored.
func Load() *Config {
	c := &Config{
		Addr:            getenv("ADDR", ":8080"),
		Env:             getenv("APP_ENV", "development"),
		DatabaseURL:     getenv("DATABASE_URL", "postgres://tripweave:tripweave@127.0.0.1:5433/tripweave?sslmode=disable"),
		RedisAddr:       getenv("REDIS_ADDR", "127.0.0.1:6380"),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		RedisTLS:        os.Getenv("REDIS_TLS") == "true",
		JWTSecret:       getenv("JWT_SECRET", defaultJWTSecret),
		AccessTTL:       parseDuration("ACCESS_TOKEN_TTL", 30*time.Minute),
		RefreshTTL:      parseDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		AIAPIKey:        os.Getenv("DEEPSEEK_API_KEY"),
		AIProvider:      getenv("AI_PROVIDER", "deepseek"),
		AIBaseURL:       getenv("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
		AIModel:         getenv("DEEPSEEK_MODEL", "deepseek-chat"),
		AITimeout:       parseDuration("AI_REQUEST_TIMEOUT", 180*time.Second),
		AIMaxToolRounds: parseInt("AI_MAX_TOOL_ROUNDS", 5),
		MapAPIKey:       os.Getenv("AMAP_SERVICE_KEY"),
		MigrateDir:      getenv("MIGRATE_DIR", "file://migrations"),
		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        os.Getenv("SMTP_PORT"),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPass:        os.Getenv("SMTP_PASS"),
		SMTPFrom:        os.Getenv("SMTP_FROM"),
		AppBaseURL:      getenv("APP_BASE_URL", "http://localhost:5273"),
	}
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		c.CORSOrigins = splitComma(v)
	} else {
		c.CORSOrigins = []string{"http://localhost:5273"}
	}
	// Auto-migrate on boot is a dev convenience; in production it must be an
	// explicit opt-in (RUN_MIGRATE=true) so rolling deploys don't race.
	c.RunMigrate = !c.IsProd()
	if v := os.Getenv("RUN_MIGRATE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.RunMigrate = b
		} else {
			slog.Warn("invalid RUN_MIGRATE value, using default", "value", v, "default", c.RunMigrate)
		}
	}
	return c
}

// Validate enforces production invariants. Callers should fail fast on error.
func (c *Config) Validate() error {
	if c.IsProd() && c.JWTSecret == defaultJWTSecret {
		return ErrInsecureJWTSecret
	}
	return nil
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		slog.Warn("invalid duration env value, using default", "key", key, "value", v, "default", fallback)
	}
	return fallback
}

func parseInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
		slog.Warn("invalid int env value, using default", "key", key, "value", v, "default", fallback)
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
