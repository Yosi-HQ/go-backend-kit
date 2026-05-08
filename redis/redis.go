package redis

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	goredis "github.com/redis/go-redis/v9"
	"github.com/yosi-hq/go-backend-kit/config"
	"github.com/yosi-hq/go-backend-kit/logger"
	"github.com/yosi-hq/go-backend-kit/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Client = goredis.Client
type Options = goredis.Options
type Cmder = goredis.Cmder

type Option func(*options)

type options struct {
	redisOptions *goredis.Options
	pingTimeout  time.Duration
	telemetry    bool
	logger       logger.Logger
	poolName     string
}

func defaultOptions() options {
	return options{
		pingTimeout: 3 * time.Second,
		telemetry:   true,
		poolName:    "default",
	}
}

func WithRedisOptions(opts *goredis.Options) Option {
	return func(o *options) {
		if opts != nil {
			o.redisOptions = opts
		}
	}
}

func WithPingTimeout(timeout time.Duration) Option {
	return func(o *options) {
		if timeout > 0 {
			o.pingTimeout = timeout
		}
	}
}

func WithTelemetry(enabled bool) Option {
	return func(o *options) {
		o.telemetry = enabled
	}
}

func WithLogger(log logger.Logger) Option {
	return func(o *options) {
		o.logger = log
	}
}

func WithPoolName(name string) Option {
	return func(o *options) {
		if name != "" {
			o.poolName = name
		}
	}
}

func NewClient(cfg config.RedisConfig, opts ...Option) (*goredis.Client, error) {
	applyDefaults(&cfg)
	o := defaultOptions()
	applyConfig(&o, cfg)
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	redisOptions := o.redisOptions
	if redisOptions == nil {
		redisOptions = &goredis.Options{
			Addr:         cfg.Addr(),
			Username:     cfg.Username,
			Password:     cfg.Password,
			DB:           cfg.DB,
			PoolSize:     cfg.PoolSize,
			MinIdleConns: cfg.MinIdleConns,
			DialTimeout:  cfg.DialTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		}
	}
	if redisOptions.Addr == "" {
		redisOptions.Addr = cfg.Addr()
	}

	client := goredis.NewClient(redisOptions)
	if o.telemetry {
		client.AddHook(newTelemetryHook(o.poolName, o.logger))
	}

	return client, nil
}

func applyDefaults(cfg *config.RedisConfig) {
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 6379
	}
	if cfg.PoolSize == 0 {
		cfg.PoolSize = 10
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 3 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 3 * time.Second
	}
	if cfg.PingTimeout == 0 {
		cfg.PingTimeout = 3 * time.Second
	}
}

func Connect(ctx context.Context, cfg config.RedisConfig, opts ...Option) (*goredis.Client, error) {
	client, err := NewClient(cfg, opts...)
	if err != nil {
		return nil, err
	}

	timeout := cfg.PingTimeout
	if timeout <= 0 {
		timeout = defaultOptions().pingTimeout
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return client, nil
}

func Ping(ctx context.Context, client *goredis.Client, timeout time.Duration) error {
	if client == nil {
		return fmt.Errorf("redis client is nil")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	return nil
}

func Close(client *goredis.Client) error {
	if client == nil {
		return nil
	}
	return client.Close()
}

func applyConfig(o *options, cfg config.RedisConfig) {
	if cfg.PingTimeout > 0 {
		o.pingTimeout = cfg.PingTimeout
	}
}

type telemetryHook struct {
	poolName string
	tracer   trace.Tracer
	logger   logger.Logger
}

func newTelemetryHook(poolName string, log logger.Logger) telemetryHook {
	if poolName == "" {
		poolName = "default"
	}

	return telemetryHook{
		poolName: poolName,
		tracer:   otel.Tracer("go-backend-kit/redis"),
		logger:   log,
	}
}

func (h telemetryHook) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, span := h.tracer.Start(ctx, "redis dial")
		start := time.Now()
		conn, err := next(ctx, network, addr)
		duration := time.Since(start)
		span.SetAttributes(
			attribute.String("net.transport", network),
			attribute.String("net.peer.name", addr),
			attribute.String("redis.pool", h.poolName),
		)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()

		if h.logger != nil {
			h.logger.DebugContext(ctx, "redis dial completed", "addr", addr, "duration_ms", duration.Milliseconds(), "error", err)
		}

		return conn, err
	}
}

func (h telemetryHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		command := normalizeCommand(cmd.FullName())
		ctx, span := h.tracer.Start(ctx, "redis "+command)
		start := time.Now()
		err := next(ctx, cmd)
		duration := time.Since(start)
		status := statusLabel(err)

		span.SetAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", command),
			attribute.String("redis.pool", h.poolName),
		)
		if err != nil && err != goredis.Nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()

		telemetry.ObserveRedisCommand(command, status, duration)
		if h.logger != nil {
			h.logger.DebugContext(ctx, "redis command completed", "command", command, "status", status, "duration_ms", duration.Milliseconds())
		}

		return err
	}
}

func (h telemetryHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		ctx, span := h.tracer.Start(ctx, "redis pipeline")
		start := time.Now()
		err := next(ctx, cmds)
		duration := time.Since(start)
		status := statusLabel(err)

		span.SetAttributes(
			attribute.String("db.system", "redis"),
			attribute.String("db.operation", "pipeline"),
			attribute.String("redis.pool", h.poolName),
			attribute.Int("redis.pipeline.commands", len(cmds)),
		)
		if err != nil && err != goredis.Nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()

		telemetry.ObserveRedisCommand("pipeline", status, duration)
		if h.logger != nil {
			h.logger.DebugContext(ctx, "redis pipeline completed", "commands", len(cmds), "status", status, "duration_ms", duration.Milliseconds())
		}

		return err
	}
}

func normalizeCommand(command string) string {
	command = strings.TrimSpace(strings.ToLower(command))
	if command == "" {
		return "unknown"
	}
	return command
}

func statusLabel(err error) string {
	if err == nil || err == goredis.Nil {
		return "ok"
	}
	return "error"
}

type PoolStatsCollector struct {
	poolName string
	client   *goredis.Client

	totalConns      *prometheus.Desc
	idleConns       *prometheus.Desc
	staleConns      *prometheus.Desc
	hits            *prometheus.Desc
	misses          *prometheus.Desc
	timeouts        *prometheus.Desc
	waitCount       *prometheus.Desc
	waitDuration    *prometheus.Desc
	pendingRequests *prometheus.Desc
}

func NewPoolStatsCollector(poolName string, client *goredis.Client) *PoolStatsCollector {
	if poolName == "" {
		poolName = "default"
	}
	labels := prometheus.Labels{"pool": poolName}

	return &PoolStatsCollector{
		poolName: poolName,
		client:   client,
		totalConns: prometheus.NewDesc(
			"redis_pool_total_connections",
			"Total number of Redis connections in the pool.",
			nil,
			labels,
		),
		idleConns: prometheus.NewDesc(
			"redis_pool_idle_connections",
			"Number of idle Redis connections in the pool.",
			nil,
			labels,
		),
		staleConns: prometheus.NewDesc(
			"redis_pool_stale_connections_total",
			"Total stale Redis connections removed from the pool.",
			nil,
			labels,
		),
		hits: prometheus.NewDesc(
			"redis_pool_hits_total",
			"Total times a free Redis connection was found.",
			nil,
			labels,
		),
		misses: prometheus.NewDesc(
			"redis_pool_misses_total",
			"Total times a free Redis connection was not found.",
			nil,
			labels,
		),
		timeouts: prometheus.NewDesc(
			"redis_pool_timeouts_total",
			"Total Redis pool timeout events.",
			nil,
			labels,
		),
		waitCount: prometheus.NewDesc(
			"redis_pool_wait_count_total",
			"Total waits for Redis pool connections.",
			nil,
			labels,
		),
		waitDuration: prometheus.NewDesc(
			"redis_pool_wait_duration_seconds_total",
			"Total time spent waiting for Redis pool connections.",
			nil,
			labels,
		),
		pendingRequests: prometheus.NewDesc(
			"redis_pool_pending_requests",
			"Current Redis pool pending requests.",
			nil,
			labels,
		),
	}
}

func (c *PoolStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalConns
	ch <- c.idleConns
	ch <- c.staleConns
	ch <- c.hits
	ch <- c.misses
	ch <- c.timeouts
	ch <- c.waitCount
	ch <- c.waitDuration
	ch <- c.pendingRequests
}

func (c *PoolStatsCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.client == nil {
		return
	}

	stats := c.client.PoolStats()
	ch <- prometheus.MustNewConstMetric(c.totalConns, prometheus.GaugeValue, float64(stats.TotalConns))
	ch <- prometheus.MustNewConstMetric(c.idleConns, prometheus.GaugeValue, float64(stats.IdleConns))
	ch <- prometheus.MustNewConstMetric(c.staleConns, prometheus.CounterValue, float64(stats.StaleConns))
	ch <- prometheus.MustNewConstMetric(c.hits, prometheus.CounterValue, float64(stats.Hits))
	ch <- prometheus.MustNewConstMetric(c.misses, prometheus.CounterValue, float64(stats.Misses))
	ch <- prometheus.MustNewConstMetric(c.timeouts, prometheus.CounterValue, float64(stats.Timeouts))
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(stats.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.waitDuration, prometheus.CounterValue, float64(stats.WaitDurationNs)/float64(time.Second))
	ch <- prometheus.MustNewConstMetric(c.pendingRequests, prometheus.GaugeValue, float64(stats.PendingRequests))
}

func RegisterPoolStatsCollector(reg prometheus.Registerer, poolName string, client *goredis.Client) error {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	if client == nil {
		return fmt.Errorf("redis client is nil")
	}

	err := reg.Register(NewPoolStatsCollector(poolName, client))
	if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
		return nil
	}
	return err
}
