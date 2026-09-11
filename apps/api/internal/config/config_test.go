package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("RUN_MIGRATE", "")
	cfg := Load()
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.AccessTTL != 30*time.Minute {
		t.Errorf("AccessTTL = %v, want 30m", cfg.AccessTTL)
	}
	if !cfg.RunMigrate {
		t.Error("RunMigrate should default to true outside production")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("dev default config should validate, got %v", err)
	}
}

func TestProdRequiresRealJWTSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "")
	cfg := Load()
	if err := cfg.Validate(); err != ErrInsecureJWTSecret {
		t.Errorf("Validate() = %v, want ErrInsecureJWTSecret", err)
	}
	if cfg.RunMigrate {
		t.Error("RunMigrate must default to false in production")
	}
}

func TestProdWithSecretValidates(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "a-real-secret")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestRunMigrateExplicitOverride(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("RUN_MIGRATE", "true")
	if !Load().RunMigrate {
		t.Error("explicit RUN_MIGRATE=true must win over the production default")
	}
}

func TestInvalidDurationFallsBack(t *testing.T) {
	t.Setenv("ACCESS_TOKEN_TTL", "not-a-duration")
	if got := Load().AccessTTL; got != 30*time.Minute {
		t.Errorf("AccessTTL = %v, want fallback 30m", got)
	}
}

func TestSplitComma(t *testing.T) {
	got := splitComma(" https://a.com , ,https://b.com ,")
	if len(got) != 2 || got[0] != "https://a.com" || got[1] != "https://b.com" {
		t.Errorf("splitComma = %v", got)
	}
}
