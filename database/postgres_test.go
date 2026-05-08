package database

import (
	"strings"
	"testing"

	"github.com/yosi-hq/go-backend-kit/config"
)

func TestBuildPostgresDSN(t *testing.T) {
	dsn, err := BuildPostgresDSN(config.PostgresConfig{
		Host:     "localhost",
		Port:     5433,
		User:     "app",
		Password: "secret",
		Database: "billing",
	}, "disable")
	if err != nil {
		t.Fatalf("BuildPostgresDSN() error = %v", err)
	}

	for _, want := range []string{
		"postgres://app:secret@localhost:5433/billing",
		"sslmode=disable",
	} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("dsn = %q, want to contain %q", dsn, want)
		}
	}
}

func TestBuildPostgresDSNRequiresFields(t *testing.T) {
	if _, err := BuildPostgresDSN(config.PostgresConfig{}, "disable"); err == nil {
		t.Fatalf("BuildPostgresDSN() error = nil, want required field error")
	}
}
