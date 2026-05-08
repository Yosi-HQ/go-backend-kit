package config

import (
	"testing"
	"time"
)

func TestLoadFromEnvironment(t *testing.T) {
	t.Setenv("APP_NAME", "billing-api")
	t.Setenv("APP_ENV", "development")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("SERVER_READ_TIMEOUT", "2s")
	t.Setenv("POSTGRES_HOST", "db")
	t.Setenv("POSTGRES_USER", "app")
	t.Setenv("POSTGRES_DATABASE", "billing")
	t.Setenv("POSTGRES_SSL_MODE", "disable")
	t.Setenv("AUTH_AUDIENCE", "api,admin")

	cfg, err := LoadWithOptions(nil, EnvOnly())
	if err != nil {
		t.Fatalf("LoadWithOptions() error = %v", err)
	}

	if cfg.App.Name != "billing-api" {
		t.Fatalf("App.Name = %q, want billing-api", cfg.App.Name)
	}
	if cfg.Server.Port != 9090 {
		t.Fatalf("Server.Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 2*time.Second {
		t.Fatalf("Server.ReadTimeout = %s, want 2s", cfg.Server.ReadTimeout)
	}
	if cfg.Postgres.Host != "db" {
		t.Fatalf("Postgres.Host = %q, want db", cfg.Postgres.Host)
	}
	if cfg.Postgres.SSLMode != "disable" {
		t.Fatalf("Postgres.SSLMode = %q, want disable", cfg.Postgres.SSLMode)
	}
	if len(cfg.Auth.Audience) != 2 || cfg.Auth.Audience[0] != "api" || cfg.Auth.Audience[1] != "admin" {
		t.Fatalf("Auth.Audience = %#v, want api/admin", cfg.Auth.Audience)
	}
}

func TestRequireEnv(t *testing.T) {
	t.Setenv("REQUIRED_VALUE", "present")

	if err := RequireEnv("REQUIRED_VALUE"); err != nil {
		t.Fatalf("RequireEnv() error = %v", err)
	}
	if err := RequireEnv("MISSING_VALUE"); err == nil {
		t.Fatalf("RequireEnv() error = nil, want missing variable error")
	}
}
