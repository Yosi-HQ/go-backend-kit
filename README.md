# go-backend-kit

Production-ready Go backend toolkit with reusable infrastructure for configuration, structured logging, PostgreSQL, GORM, Redis, JWT auth, Gin middleware, HTTP server lifecycle, and observability.

## Features

- Typed environment configuration with `.env` support
- JSON structured logging built on `log/slog`
- PostgreSQL `database/sql` setup with pool tuning and health checks
- GORM setup with query spans and database metrics
- Redis setup with ping checks, command traces, command metrics, and pool metrics
- Gin middleware for logging, recovery, CORS, rate limiting, JWT auth, request IDs, and trace IDs
- JWT generation, validation, claims parsing, roles, and scopes
- OpenTelemetry traces and metrics with optional OTLP export
- Prometheus metrics endpoint support
- Graceful HTTP startup and shutdown helpers

## Installation

```bash
go get github.com/yosi-hq/go-backend-kit
```

## Quick Start

```go
package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yosi-hq/go-backend-kit/config"
	"github.com/yosi-hq/go-backend-kit/database"
	"github.com/yosi-hq/go-backend-kit/logger"
	"github.com/yosi-hq/go-backend-kit/middleware"
	kitredis "github.com/yosi-hq/go-backend-kit/redis"
	"github.com/yosi-hq/go-backend-kit/server"
	"github.com/yosi-hq/go-backend-kit/telemetry"
)

func main() {
	ctx := context.Background()

	cfg, err := config.LoadWithOptions([]string{"."}, config.WithValidation())
	if err != nil {
		panic(err)
	}

	log := logger.New(cfg.App.Env, logger.WithAttrs("service", cfg.App.Name))

	otel, err := telemetry.Init(ctx, telemetry.Config{
		ServiceName:       cfg.Telemetry.ServiceName,
		Environment:       cfg.Telemetry.Environment,
		Version:           cfg.Telemetry.Version,
		OTLPEndpoint:      cfg.Telemetry.OTLPEndpoint,
		OTLPInsecure:      cfg.Telemetry.OTLPInsecure,
		TracesEnabled:     cfg.Telemetry.TracesEnabled,
		MetricsEnabled:    cfg.Telemetry.MetricsEnabled,
		PrometheusEnabled: cfg.Telemetry.PrometheusEnabled,
		SampleRatio:       cfg.Telemetry.SampleRatio,
	})
	if err != nil {
		log.Error("telemetry setup failed", "error", err)
	}
	if otel != nil {
		defer otel.Shutdown(ctx)
	}

	db, err := database.ConnectGorm(ctx, cfg.Postgres)
	if err != nil {
		log.Error("database connection failed", "error", err)
		panic(err)
	}
	defer database.CloseGorm(db)

	sqlDB, err := database.DBFromGorm(db)
	if err == nil {
		_ = database.RegisterSQLStatsCollector(nil, "main", sqlDB)
	}

	redisClient, err := kitredis.Connect(ctx, cfg.Redis, kitredis.WithLogger(log))
	if err != nil {
		log.Error("redis connection failed", "error", err)
		panic(err)
	}
	defer kitredis.Close(redisClient)
	_ = kitredis.RegisterPoolStatsCollector(nil, "main", redisClient)

	router := gin.New()
	router.Use(
		middleware.RecoveryMiddleware(log),
		middleware.LoggerMiddlewareWith(log),
		middleware.TelemetryMiddleware(cfg.Telemetry.ServiceName),
		middleware.CORS(),
		middleware.RateLimit(120, time.Minute),
	)

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET(cfg.Telemetry.PrometheusPath, gin.WrapH(telemetry.MetricsHandler()))

	runCtx, stop := server.ContextWithSignals(ctx)
	defer stop()

	httpServer := server.New(cfg.Server, router, server.WithLogger(log))
	if err := httpServer.Run(runCtx); err != nil {
		log.Error("server stopped", "error", err)
	}
}
```

## Configuration

`config.Load(".")` loads `./.env` first and then overlays real environment variables. Env names use the package section first and the field name after it:

```env
APP_NAME=billing-api
APP_ENV=production
APP_VERSION=1.0.0

SERVER_HOST=0.0.0.0
SERVER_PORT=8080
SERVER_READ_TIMEOUT=10s
SERVER_WRITE_TIMEOUT=30s
SERVER_IDLE_TIMEOUT=120s
SERVER_SHUTDOWN_TIMEOUT=15s

POSTGRES_ENABLED=true
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=app
POSTGRES_PASSWORD=secret
POSTGRES_DATABASE=appdb
POSTGRES_SSL_MODE=require
POSTGRES_MAX_OPEN_CONNS=25
POSTGRES_MAX_IDLE_CONNS=25

REDIS_ENABLED=true
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

AUTH_JWT_SECRET=change-me-to-at-least-32-bytes-long
AUTH_ISSUER=billing-api
AUTH_AUDIENCE=api
AUTH_ACCESS_TTL=15m

TELEMETRY_SERVICE_NAME=billing-api
TELEMETRY_OTLP_ENDPOINT=localhost:4317
TELEMETRY_OTLP_INSECURE=true
TELEMETRY_PROMETHEUS_PATH=/metrics
```

For local Docker PostgreSQL, set `POSTGRES_SSL_MODE=disable`.

Useful config APIs:

```go
cfg, err := config.Load(".")
cfg, err = config.LoadWithOptions([]string{"."}, config.WithRequiredEnv("AUTH_JWT_SECRET"))
cfg = config.MustLoad(".")
addr := cfg.Server.Addr()
```

## Packages

### Logger

```go
log := logger.New("production")
log.Info("application started", "service", "billing-api")
requestLog := log.With("request_id", "req-123")
requestLog.Error("operation failed", "error", err)
```

The legacy `github.com/yosi-hq/go-backend-kit/log` package remains as an alias for compatibility. New code should import `logger`.

### Database

```go
sqlDB, err := database.ConnectPostgres(ctx, cfg.Postgres)
defer database.Close(sqlDB)

gormDB, err := database.ConnectGorm(ctx, cfg.Postgres)
defer database.CloseGorm(gormDB)
```

`ConnectGorm` installs a GORM telemetry plugin by default. It records spans and Prometheus metrics for `create`, `query`, `update`, `delete`, `raw`, and `row` operations.

```go
gormDB, err := database.ConnectGorm(
	ctx,
	cfg.Postgres,
	database.WithGormSQLInSpans(false),
	database.WithPostgresOptions(database.WithPostgresMaxOpenConns(50)),
)
```

Only enable SQL text in spans when it is safe for your data:

```go
gormDB, err := database.ConnectGorm(ctx, cfg.Postgres, database.WithGormSQLInSpans(true))
```

Pool metrics:

```go
sqlDB, _ := database.DBFromGorm(gormDB)
_ = database.RegisterSQLStatsCollector(nil, "main", sqlDB)
```

### Redis

```go
client, err := redis.Connect(ctx, cfg.Redis, redis.WithLogger(log))
defer redis.Close(client)

if err := redis.Ping(ctx, client, 3*time.Second); err != nil {
	log.Error("redis health check failed", "error", err)
}

_ = redis.RegisterPoolStatsCollector(nil, "main", client)
```

Redis command and pipeline spans are enabled by default. Disable them when needed:

```go
client, err := redis.Connect(ctx, cfg.Redis, redis.WithTelemetry(false))
```

### Auth

```go
token, err := auth.GenerateTokenWithClaims(auth.JWTConfig{
	Secret:    cfg.Auth.JWTSecret,
	Issuer:    cfg.Auth.Issuer,
	Audience:  cfg.Auth.Audience,
	AccessTTL: cfg.Auth.AccessTTL,
}, auth.Claims{
	Subject: "user-123",
	Roles:   []string{"admin"},
	Scopes:  []string{"users:read"},
})

claims, err := auth.ValidateTokenWithConfig(auth.JWTConfig{
	Secret:   cfg.Auth.JWTSecret,
	Issuer:   cfg.Auth.Issuer,
	Audience: cfg.Auth.Audience,
	Leeway:   cfg.Auth.Leeway,
}, token)
```

Role and scope helpers:

```go
if auth.HasRole(claims, "admin") && auth.HasScope(claims, "users:read") {
	// authorized
}
```

### Middleware

```go
router := gin.New()
router.Use(
	middleware.RecoveryMiddleware(log),
	middleware.LoggerMiddlewareWith(log),
	middleware.TelemetryMiddleware(cfg.Telemetry.ServiceName),
	middleware.CORS(),
	middleware.RateLimit(100, time.Minute),
)

private := router.Group("/api")
private.Use(middleware.JWTAuthWithConfig(auth.JWTConfig{
	Secret:   cfg.Auth.JWTSecret,
	Issuer:   cfg.Auth.Issuer,
	Audience: cfg.Auth.Audience,
	Leeway:   cfg.Auth.Leeway,
}))
private.GET("/users", middleware.RequireScopes("users:read"), listUsers)
```

Middleware adds:

- `X-Request-Id` for request correlation
- `X-Trace-Id` for distributed tracing correlation
- structured request logs with status, latency, bytes, request ID, and trace ID
- panic recovery with stack traces in logs

### Telemetry

```go
handle, err := telemetry.Init(ctx, telemetry.Config{
	ServiceName:       "billing-api",
	Environment:       "production",
	OTLPEndpoint:      "localhost:4317",
	OTLPInsecure:      true,
	TracesEnabled:     true,
	MetricsEnabled:    true,
	PrometheusEnabled: true,
})
if err != nil {
	panic(err)
}
defer handle.Shutdown(ctx)

router.GET("/metrics", gin.WrapH(telemetry.MetricsHandler()))
```

Observability coverage:

- HTTP spans and metrics from `middleware.TelemetryMiddleware`
- request IDs and trace IDs in logs from `middleware.LoggerMiddlewareWith`
- GORM query spans and `go_backend_kit_db_*` metrics
- Redis command spans and `go_backend_kit_redis_*` metrics
- SQL pool metrics from `database.RegisterSQLStatsCollector`
- Redis pool metrics from `redis.RegisterPoolStatsCollector`

### Server

```go
ctx, stop := server.ContextWithSignals(context.Background())
defer stop()

srv := server.New(cfg.Server, router, server.WithLogger(log))
if err := srv.Run(ctx); err != nil {
	log.Error("server stopped", "error", err)
}
```

`server.Run` listens until the context is cancelled, then calls graceful shutdown with `SERVER_SHUTDOWN_TIMEOUT`.

## Design Principles

- Keep packages small and focused
- Prefer explicit code over magic abstractions
- Use secure defaults where practical
- Keep operational signals close to the work: logs, traces, metrics, health checks
- Avoid logging secrets or raw SQL unless explicitly enabled
- Make common service setup reusable without hiding the underlying Go libraries

## Development

```bash
go test ./...
```

## Contributing

Contributions, issues, and ideas are welcome.
