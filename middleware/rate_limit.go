package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type RateLimitConfig struct {
	Limit  int
	Window time.Duration
	KeyFn  func(*gin.Context) string
}

type rateBucket struct {
	count     int
	resetTime time.Time
}

func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	return RateLimitWithConfig(RateLimitConfig{Limit: limit, Window: window})
}

func RateLimitWithConfig(cfg RateLimitConfig) gin.HandlerFunc {
	if cfg.Limit <= 0 {
		cfg.Limit = 60
	}
	if cfg.Window <= 0 {
		cfg.Window = time.Minute
	}
	if cfg.KeyFn == nil {
		cfg.KeyFn = func(c *gin.Context) string { return c.ClientIP() }
	}

	var mu sync.Mutex
	buckets := make(map[string]rateBucket)

	return func(c *gin.Context) {
		now := time.Now()
		key := cfg.KeyFn(c)

		mu.Lock()
		bucket := buckets[key]
		if bucket.resetTime.IsZero() || now.After(bucket.resetTime) {
			bucket = rateBucket{resetTime: now.Add(cfg.Window)}
		}
		bucket.count++
		buckets[key] = bucket

		remaining := cfg.Limit - bucket.count
		if remaining < 0 {
			remaining = 0
		}
		resetUnix := bucket.resetTime.Unix()
		limited := bucket.count > cfg.Limit
		if len(buckets) > 10000 {
			for bucketKey, value := range buckets {
				if now.After(value.resetTime) {
					delete(buckets, bucketKey)
				}
			}
		}
		mu.Unlock()

		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetUnix, 10))

		if limited {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}

		c.Next()
	}
}
