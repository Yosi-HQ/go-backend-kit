package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrMissingSecret = errors.New("jwt secret is required")
	ErrInvalidToken  = errors.New("jwt token is invalid")
	ErrExpiredToken  = errors.New("jwt token is expired")
)

type JWTConfig struct {
	Secret    string
	Issuer    string
	Audience  []string
	AccessTTL time.Duration
	Leeway    time.Duration
}

type Claims struct {
	Subject   string
	Roles     []string
	Scopes    []string
	Issuer    string
	Audience  []string
	ExpiresAt time.Time
	NotBefore time.Time
	IssuedAt  time.Time
	ID        string
	Extra     map[string]any
}

func GenerateToken(secret string, subject string, roles []string, ttl time.Duration) (string, error) {
	return GenerateTokenWithClaims(JWTConfig{Secret: secret, AccessTTL: ttl}, Claims{
		Subject: subject,
		Roles:   roles,
	})
}

func GenerateTokenWithClaims(cfg JWTConfig, claims Claims) (string, error) {
	if cfg.Secret == "" {
		return "", ErrMissingSecret
	}
	if claims.Issuer == "" {
		claims.Issuer = cfg.Issuer
	}
	if len(claims.Audience) == 0 {
		claims.Audience = cfg.Audience
	}
	if claims.IssuedAt.IsZero() {
		claims.IssuedAt = time.Now().UTC()
	}
	if claims.ExpiresAt.IsZero() && cfg.AccessTTL > 0 {
		claims.ExpiresAt = claims.IssuedAt.Add(cfg.AccessTTL)
	}

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims.toMap())
	if err != nil {
		return "", err
	}

	unsigned := encodeSegment(headerJSON) + "." + encodeSegment(claimsJSON)
	signature := signHS256(unsigned, cfg.Secret)

	return unsigned + "." + encodeSegment(signature), nil
}

func ValidateToken(secret string, token string) (*Claims, error) {
	return ValidateTokenWithConfig(JWTConfig{Secret: secret}, token)
}

func ValidateTokenWithConfig(cfg JWTConfig, token string) (*Claims, error) {
	if cfg.Secret == "" {
		return nil, ErrMissingSecret
	}
	if cfg.Leeway < 0 {
		cfg.Leeway = 0
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	headerBytes, err := decodeSegment(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: bad header", ErrInvalidToken)
	}

	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("%w: bad header json", ErrInvalidToken)
	}
	if alg, _ := header["alg"].(string); alg != "HS256" {
		return nil, fmt.Errorf("%w: unsupported alg", ErrInvalidToken)
	}

	unsigned := parts[0] + "." + parts[1]
	expected := signHS256(unsigned, cfg.Secret)
	actual, err := decodeSegment(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: bad signature", ErrInvalidToken)
	}
	if !hmac.Equal(expected, actual) {
		return nil, fmt.Errorf("%w: signature mismatch", ErrInvalidToken)
	}

	payload, err := decodeSegment(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: bad payload", ErrInvalidToken)
	}

	claims, err := claimsFromJSON(payload)
	if err != nil {
		return nil, err
	}
	if err := validateClaims(cfg, claims, time.Now().UTC()); err != nil {
		return nil, err
	}

	return claims, nil
}

func (c Claims) toMap() map[string]any {
	claims := make(map[string]any, len(c.Extra)+10)
	for key, value := range c.Extra {
		claims[key] = value
	}
	if c.Subject != "" {
		claims["sub"] = c.Subject
	}
	if len(c.Roles) > 0 {
		claims["roles"] = c.Roles
	}
	if len(c.Scopes) > 0 {
		claims["scopes"] = c.Scopes
	}
	if c.Issuer != "" {
		claims["iss"] = c.Issuer
	}
	if len(c.Audience) == 1 {
		claims["aud"] = c.Audience[0]
	} else if len(c.Audience) > 1 {
		claims["aud"] = c.Audience
	}
	if !c.ExpiresAt.IsZero() {
		claims["exp"] = c.ExpiresAt.Unix()
	}
	if !c.NotBefore.IsZero() {
		claims["nbf"] = c.NotBefore.Unix()
	}
	if !c.IssuedAt.IsZero() {
		claims["iat"] = c.IssuedAt.Unix()
	}
	if c.ID != "" {
		claims["jti"] = c.ID
	}
	return claims
}

func claimsFromJSON(payload []byte) (*Claims, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("%w: bad payload json", ErrInvalidToken)
	}

	claims := &Claims{Extra: make(map[string]any)}
	for key, value := range raw {
		switch key {
		case "sub":
			claims.Subject, _ = value.(string)
		case "roles":
			claims.Roles = stringSlice(value)
		case "scopes", "scope":
			claims.Scopes = scopes(value)
		case "iss":
			claims.Issuer, _ = value.(string)
		case "aud":
			claims.Audience = audience(value)
		case "exp":
			claims.ExpiresAt = unixTime(value)
		case "nbf":
			claims.NotBefore = unixTime(value)
		case "iat":
			claims.IssuedAt = unixTime(value)
		case "jti":
			claims.ID, _ = value.(string)
		default:
			claims.Extra[key] = value
		}
	}

	return claims, nil
}

func validateClaims(cfg JWTConfig, claims *Claims, now time.Time) error {
	if !claims.ExpiresAt.IsZero() && now.After(claims.ExpiresAt.Add(cfg.Leeway)) {
		return ErrExpiredToken
	}
	if !claims.NotBefore.IsZero() && now.Add(cfg.Leeway).Before(claims.NotBefore) {
		return fmt.Errorf("%w: token not valid yet", ErrInvalidToken)
	}
	if !claims.IssuedAt.IsZero() && now.Add(cfg.Leeway).Before(claims.IssuedAt) {
		return fmt.Errorf("%w: token issued in the future", ErrInvalidToken)
	}
	if cfg.Issuer != "" && claims.Issuer != cfg.Issuer {
		return fmt.Errorf("%w: issuer mismatch", ErrInvalidToken)
	}
	if len(cfg.Audience) > 0 && !audienceMatches(cfg.Audience, claims.Audience) {
		return fmt.Errorf("%w: audience mismatch", ErrInvalidToken)
	}

	return nil
}

func encodeSegment(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeSegment(segment string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(segment)
}

func signHS256(unsigned string, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	return mac.Sum(nil)
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

func scopes(value any) []string {
	if s, ok := value.(string); ok {
		return strings.Fields(s)
	}
	return stringSlice(value)
}

func audience(value any) []string {
	return stringSlice(value)
}

func unixTime(value any) time.Time {
	switch v := value.(type) {
	case float64:
		return time.Unix(int64(v), 0).UTC()
	case int64:
		return time.Unix(v, 0).UTC()
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return time.Unix(n, 0).UTC()
		}
	}
	return time.Time{}
}

func audienceMatches(required []string, actual []string) bool {
	for _, want := range required {
		for _, got := range actual {
			if want == got {
				return true
			}
		}
	}
	return false
}
