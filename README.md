# go-backend-kit

Production-ready Go backend toolkit with reusable infrastructure components for configuration, logging, database initialization, Redis, authentication, middleware, and observability.

---

## Features

- Structured logging
- Environment-based configuration
- PostgreSQL initialization
- Redis initialization
- HTTP server setup
- Middleware support
- JWT authentication utilities
- Graceful shutdown
- Request tracing and observability
- Scalable project structure
- Reusable backend infrastructure

---

## Goals

The goal of this project is to provide reusable and maintainable backend infrastructure for Go services without rewriting common setup logic for every project.

This repository focuses on:

- Clean architecture
- Modularity
- Observability
- Security
- Production-ready patterns
- Developer experience

---



## Installation

```bash
go get github.com/YOUR_USERNAME/go-backend-kit
```

---

## Quick Example

```go
package main

import (
    "github.com/YOUR_USERNAME/go-backend-kit/config"
    "github.com/YOUR_USERNAME/go-backend-kit/database"
    "github.com/YOUR_USERNAME/go-backend-kit/logger"
)

func main() {
    cfg := config.Load()

    log := logger.New()

    db, err := database.Connect(cfg.Database)
    if err != nil {
        log.Error("failed to connect database", "error", err)
        panic(err)
    }

    log.Info("application started")

    _ = db
}
```

---

## Planned Packages

### Config

Environment loading and configuration management.

Features:

* `.env` support
* Typed config structs
* Required environment validation

---

### Logger

Structured logging utilities.

Features:

* JSON logs
* Log levels
* Request IDs
* Contextual logging

---

### Database

PostgreSQL and GORM initialization.

Features:

* Connection pooling
* Timeout configuration
* Health checks
* Graceful shutdown support

---

### Redis

Redis client initialization and utilities.

Features:

* Connection management
* Health checks
* Timeout support

---

### Middleware

Reusable HTTP middleware for Gin.

Features:

* Logging
* Recovery
* CORS
* Rate limiting
* Authentication middleware

---

### Auth

Authentication and authorization utilities.

Features:

* JWT generation
* JWT validation
* Claims parsing
* RBAC helpers

---

### Telemetry

Observability and monitoring.

Features:

* OpenTelemetry integration
* Tracing
* Prometheus metrics
* Request duration tracking

---

## Design Principles

* Keep packages small and focused
* Prefer explicit code over magic abstractions
* Production-first engineering
* Secure defaults
* Minimal dependencies
* Reusable but not over-engineered

---

## Security

Security is a core consideration for this project.

Practices include:

* Secure defaults
* Timeout enforcement
* Structured logging
* Minimal dependency usage
* Environment-based secret management
* Vulnerability scanning support

---

## Future Improvements

* Kafka/NATS integration
* Distributed tracing examples
* RBAC system
* Audit logging
* CLI utilities
* Docker and Kubernetes support
* CI/CD templates

---

## Contributing

Contributions, ideas, and improvements are welcome.

---

