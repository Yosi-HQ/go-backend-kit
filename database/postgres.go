package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/yosi-hq/go-backend-kit/config"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PostgresOption func(*postgresOptions)

type postgresOptions struct {
	sslMode         string
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	connMaxIdleTime time.Duration
	pingTimeout     time.Duration
	tracingEnabled  bool
}

func defaultPostgresOptions() postgresOptions {
	return postgresOptions{
		sslMode:         "require",
		maxOpenConns:    25,
		maxIdleConns:    25,
		connMaxLifetime: 60 * time.Minute,
		connMaxIdleTime: 5 * time.Minute,
		pingTimeout:     5 * time.Second,
		tracingEnabled:  true,
	}
}

func WithPostgresSSLMode(mode string) PostgresOption {
	return func(o *postgresOptions) {
		if mode != "" {
			o.sslMode = mode
		}
	}
}

func WithPostgresMaxOpenConns(n int) PostgresOption {
	return func(o *postgresOptions) {
		if n > 0 {
			o.maxOpenConns = n
		}
	}
}

func WithPostgresMaxIdleConns(n int) PostgresOption {
	return func(o *postgresOptions) {
		if n > 0 {
			o.maxIdleConns = n
		}
	}
}

func WithPostgresConnMaxLifetime(d time.Duration) PostgresOption {
	return func(o *postgresOptions) {
		if d > 0 {
			o.connMaxLifetime = d
		}
	}
}

func WithPostgresConnMaxIdleTime(d time.Duration) PostgresOption {
	return func(o *postgresOptions) {
		if d > 0 {
			o.connMaxIdleTime = d
		}
	}
}

func WithPostgresPingTimeout(d time.Duration) PostgresOption {
	return func(o *postgresOptions) {
		if d > 0 {
			o.pingTimeout = d
		}
	}
}

// WithPostgresTracing toggles OpenTelemetry instrumentation for database/sql.
func WithPostgresTracing(enabled bool) PostgresOption {
	return func(o *postgresOptions) {
		o.tracingEnabled = enabled
	}
}

func postgresDSN(cfg config.PostgresConfig, sslMode string) (string, error) {
	return BuildPostgresDSN(cfg, sslMode)
}

func BuildPostgresDSN(cfg config.PostgresConfig, sslMode string) (string, error) {
	if cfg.Host == "" {
		return "", fmt.Errorf("postgres host is required")
	}
	if cfg.User == "" {
		return "", fmt.Errorf("postgres user is required")
	}
	if cfg.Database == "" {
		return "", fmt.Errorf("postgres database is required")
	}
	port := cfg.Port
	if port == 0 {
		port = 5432
	}
	if sslMode == "" {
		sslMode = "require"
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", cfg.Host, port),
		Path:   cfg.Database,
	}
	if cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	} else {
		u.User = url.User(cfg.User)
	}

	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// ConnectPostgres opens a PostgreSQL database/sql connection and verifies it with Ping.
func ConnectPostgres(ctx context.Context, cfg config.PostgresConfig, opts ...PostgresOption) (*sql.DB, error) {
	o := defaultPostgresOptions()
	applyConfigOptions(&o, cfg)
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	dsn, err := postgresDSN(cfg, o.sslMode)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open failed: %w", err)
	}

	// Configure connection pool.
	if o.maxOpenConns > 0 {
		db.SetMaxOpenConns(o.maxOpenConns)
	}
	if o.maxIdleConns > 0 {
		db.SetMaxIdleConns(o.maxIdleConns)
	}
	if o.connMaxLifetime > 0 {
		db.SetConnMaxLifetime(o.connMaxLifetime)
	}
	if o.connMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(o.connMaxIdleTime)
	}

	pingCtx, cancel := context.WithTimeout(ctx, o.pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping failed: %w", err)
	}

	return db, nil
}

func applyConfigOptions(o *postgresOptions, cfg config.PostgresConfig) {
	if cfg.SSLMode != "" {
		o.sslMode = cfg.SSLMode
	}
	if cfg.MaxOpenConns > 0 {
		o.maxOpenConns = cfg.MaxOpenConns
	}
	if cfg.MaxIdleConns > 0 {
		o.maxIdleConns = cfg.MaxIdleConns
	}
	if cfg.ConnMaxLifetime > 0 {
		o.connMaxLifetime = cfg.ConnMaxLifetime
	}
	if cfg.ConnMaxIdleTime > 0 {
		o.connMaxIdleTime = cfg.ConnMaxIdleTime
	}
	if cfg.PingTimeout > 0 {
		o.pingTimeout = cfg.PingTimeout
	}
}

// Ping verifies that the database is reachable within the supplied timeout.
func Ping(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	if db == nil {
		return fmt.Errorf("postgres db is nil")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	return nil
}

func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}

type SQLStatsCollector struct {
	poolName string
	db       *sql.DB

	openConnections *prometheus.Desc
	inUse           *prometheus.Desc
	idle            *prometheus.Desc
	waitCount       *prometheus.Desc
	waitDuration    *prometheus.Desc
	maxOpen         *prometheus.Desc
}

func NewSQLStatsCollector(poolName string, db *sql.DB) *SQLStatsCollector {
	if poolName == "" {
		poolName = "default"
	}

	labels := prometheus.Labels{"pool": poolName}
	return &SQLStatsCollector{
		poolName: poolName,
		db:       db,
		openConnections: prometheus.NewDesc(
			"db_pool_open_connections",
			"Number of open database connections.",
			nil,
			labels,
		),
		inUse: prometheus.NewDesc(
			"db_pool_in_use_connections",
			"Number of database connections currently in use.",
			nil,
			labels,
		),
		idle: prometheus.NewDesc(
			"db_pool_idle_connections",
			"Number of idle database connections.",
			nil,
			labels,
		),
		waitCount: prometheus.NewDesc(
			"db_pool_wait_count_total",
			"Total number of database connection waits.",
			nil,
			labels,
		),
		waitDuration: prometheus.NewDesc(
			"db_pool_wait_duration_seconds_total",
			"Total time blocked waiting for a database connection.",
			nil,
			labels,
		),
		maxOpen: prometheus.NewDesc(
			"db_pool_max_open_connections",
			"Configured maximum number of open database connections.",
			nil,
			labels,
		),
	}
}

func (c *SQLStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.openConnections
	ch <- c.inUse
	ch <- c.idle
	ch <- c.waitCount
	ch <- c.waitDuration
	ch <- c.maxOpen
}

func (c *SQLStatsCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.db == nil {
		return
	}

	stats := c.db.Stats()
	ch <- prometheus.MustNewConstMetric(c.openConnections, prometheus.GaugeValue, float64(stats.OpenConnections))
	ch <- prometheus.MustNewConstMetric(c.inUse, prometheus.GaugeValue, float64(stats.InUse))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(stats.Idle))
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(stats.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.waitDuration, prometheus.CounterValue, stats.WaitDuration.Seconds())
	ch <- prometheus.MustNewConstMetric(c.maxOpen, prometheus.GaugeValue, float64(stats.MaxOpenConnections))
}

func RegisterSQLStatsCollector(reg prometheus.Registerer, poolName string, db *sql.DB) error {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	if db == nil {
		return fmt.Errorf("postgres db is nil")
	}

	err := reg.Register(NewSQLStatsCollector(poolName, db))
	if already, ok := err.(prometheus.AlreadyRegisteredError); ok && already.ExistingCollector != nil {
		return nil
	}
	return err
}
