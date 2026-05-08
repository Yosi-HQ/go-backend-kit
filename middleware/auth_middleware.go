package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yosi-hq/go-backend-kit/auth"
)

const ClaimsKey = "auth_claims"

func JWTAuth(secret string) gin.HandlerFunc {
	return JWTAuthWithConfig(auth.JWTConfig{Secret: secret})
}

func JWTAuthWithConfig(cfg auth.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		claims, err := auth.ValidateTokenWithConfig(cfg, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid bearer token"})
			return
		}

		c.Set(ClaimsKey, claims)
		c.Next()
	}
}

func RequireRoles(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := ClaimsFromContext(c)
		if !auth.HasAnyRole(claims, roles...) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing required role"})
			return
		}
		c.Next()
	}
}

func RequireScopes(scopes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := ClaimsFromContext(c)
		if !auth.HasAnyScope(claims, scopes...) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing required scope"})
			return
		}
		c.Next()
	}
}

func ClaimsFromContext(c *gin.Context) *auth.Claims {
	if c == nil {
		return nil
	}
	value, ok := c.Get(ClaimsKey)
	if !ok {
		return nil
	}
	claims, _ := value.(*auth.Claims)
	return claims
}

func bearerToken(header string) string {
	const prefix = "bearer "
	header = strings.TrimSpace(header)
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
