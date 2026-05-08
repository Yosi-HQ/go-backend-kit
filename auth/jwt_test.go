package auth

import (
	"errors"
	"testing"
	"time"
)

func TestGenerateAndValidateToken(t *testing.T) {
	cfg := JWTConfig{
		Secret:    "01234567890123456789012345678901",
		Issuer:    "go-backend-kit",
		Audience:  []string{"api"},
		AccessTTL: time.Minute,
		Leeway:    time.Second,
	}

	token, err := GenerateTokenWithClaims(cfg, Claims{
		Subject: "user-1",
		Roles:   []string{"admin"},
		Scopes:  []string{"users:read"},
		Extra: map[string]any{
			"tenant_id": "tenant-1",
		},
	})
	if err != nil {
		t.Fatalf("GenerateTokenWithClaims() error = %v", err)
	}

	claims, err := ValidateTokenWithConfig(cfg, token)
	if err != nil {
		t.Fatalf("ValidateTokenWithConfig() error = %v", err)
	}

	if claims.Subject != "user-1" {
		t.Fatalf("Subject = %q, want user-1", claims.Subject)
	}
	if !HasRole(claims, "admin") {
		t.Fatalf("expected admin role")
	}
	if !HasScope(claims, "users:read") {
		t.Fatalf("expected users:read scope")
	}
	if claims.Extra["tenant_id"] != "tenant-1" {
		t.Fatalf("tenant_id = %v, want tenant-1", claims.Extra["tenant_id"])
	}
}

func TestValidateTokenRejectsExpiredToken(t *testing.T) {
	cfg := JWTConfig{
		Secret: "01234567890123456789012345678901",
		Leeway: time.Millisecond,
	}

	token, err := GenerateTokenWithClaims(cfg, Claims{
		Subject:   "user-1",
		ExpiresAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("GenerateTokenWithClaims() error = %v", err)
	}

	_, err = ValidateTokenWithConfig(cfg, token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("ValidateTokenWithConfig() error = %v, want ErrExpiredToken", err)
	}
}

func TestValidateTokenRejectsWrongAudience(t *testing.T) {
	secret := "01234567890123456789012345678901"
	token, err := GenerateTokenWithClaims(JWTConfig{
		Secret:    secret,
		Audience:  []string{"internal"},
		AccessTTL: time.Minute,
	}, Claims{Subject: "user-1"})
	if err != nil {
		t.Fatalf("GenerateTokenWithClaims() error = %v", err)
	}

	_, err = ValidateTokenWithConfig(JWTConfig{
		Secret:   secret,
		Audience: []string{"public"},
	}, token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateTokenWithConfig() error = %v, want ErrInvalidToken", err)
	}
}
