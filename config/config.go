package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	App       AppConfig       `mapstructure:"APP"`
	Server    ServerConfig    `mapstructure:"SERVER"`
	Postgres  PostgresConfig  `mapstructure:"POSTGRES"`
	Database  PostgresConfig  `mapstructure:"DATABASE"`
	Redis     RedisConfig     `mapstructure:"REDIS"`
	Auth      AuthConfig      `mapstructure:"AUTH"`
	Telemetry TelemetryConfig `mapstructure:"TELEMETRY"`
}

type AppConfig struct {
	Name    string `mapstructure:"NAME"`
	Env     string `mapstructure:"ENV"`
	Version string `mapstructure:"VERSION"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"HOST"`
	Port            int           `mapstructure:"PORT"`
	Env             string        `mapstructure:"ENV"`
	ReadTimeout     time.Duration `mapstructure:"READ_TIMEOUT"`
	WriteTimeout    time.Duration `mapstructure:"WRITE_TIMEOUT"`
	IdleTimeout     time.Duration `mapstructure:"IDLE_TIMEOUT"`
	ShutdownTimeout time.Duration `mapstructure:"SHUTDOWN_TIMEOUT"`
}

type PostgresConfig struct {
	Enabled         bool          `mapstructure:"ENABLED"`
	Host            string        `mapstructure:"HOST"`
	User            string        `mapstructure:"USER"`
	Password        string        `mapstructure:"PASSWORD"`
	Database        string        `mapstructure:"DATABASE"`
	Port            int           `mapstructure:"PORT"`
	SSLMode         string        `mapstructure:"SSL_MODE"`
	MaxOpenConns    int           `mapstructure:"MAX_OPEN_CONNS"`
	MaxIdleConns    int           `mapstructure:"MAX_IDLE_CONNS"`
	ConnMaxLifetime time.Duration `mapstructure:"CONN_MAX_LIFETIME"`
	ConnMaxIdleTime time.Duration `mapstructure:"CONN_MAX_IDLE_TIME"`
	PingTimeout     time.Duration `mapstructure:"PING_TIMEOUT"`
}

type RedisConfig struct {
	Enabled      bool          `mapstructure:"ENABLED"`
	Host         string        `mapstructure:"HOST"`
	Port         int           `mapstructure:"PORT"`
	Username     string        `mapstructure:"USERNAME"`
	Password     string        `mapstructure:"PASSWORD"`
	DB           int           `mapstructure:"DB"`
	PoolSize     int           `mapstructure:"POOL_SIZE"`
	MinIdleConns int           `mapstructure:"MIN_IDLE_CONNS"`
	DialTimeout  time.Duration `mapstructure:"DIAL_TIMEOUT"`
	ReadTimeout  time.Duration `mapstructure:"READ_TIMEOUT"`
	WriteTimeout time.Duration `mapstructure:"WRITE_TIMEOUT"`
	PingTimeout  time.Duration `mapstructure:"PING_TIMEOUT"`
}

type AuthConfig struct {
	JWTSecret  string        `mapstructure:"JWT_SECRET"`
	Issuer     string        `mapstructure:"ISSUER"`
	Audience   []string      `mapstructure:"AUDIENCE"`
	AccessTTL  time.Duration `mapstructure:"ACCESS_TTL"`
	RefreshTTL time.Duration `mapstructure:"REFRESH_TTL"`
	Leeway     time.Duration `mapstructure:"LEEWAY"`
}

type TelemetryConfig struct {
	Enabled           bool    `mapstructure:"ENABLED"`
	ServiceName       string  `mapstructure:"SERVICE_NAME"`
	Environment       string  `mapstructure:"ENVIRONMENT"`
	Version           string  `mapstructure:"VERSION"`
	OTLPEndpoint      string  `mapstructure:"OTLP_ENDPOINT"`
	OTLPInsecure      bool    `mapstructure:"OTLP_INSECURE"`
	TracesEnabled     bool    `mapstructure:"TRACES_ENABLED"`
	MetricsEnabled    bool    `mapstructure:"METRICS_ENABLED"`
	PrometheusEnabled bool    `mapstructure:"PROMETHEUS_ENABLED"`
	PrometheusPath    string  `mapstructure:"PROMETHEUS_PATH"`
	SampleRatio       float64 `mapstructure:"SAMPLE_RATIO"`
}

type LoadOption func(*loadOptions)

type loadOptions struct {
	paths     []string
	required  []string
	envPrefix string
	validate  bool
	fileName  string
	envOnly   bool
}

func defaultLoadOptions(paths []string) loadOptions {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	return loadOptions{
		paths:    paths,
		fileName: ".env",
	}
}

// WithRequiredEnv validates that the named environment variables exist.
func WithRequiredEnv(keys ...string) LoadOption {
	return func(o *loadOptions) {
		o.required = append(o.required, keys...)
	}
}

// WithEnvPrefix loads only environment variables with the given prefix.
//
// For example, WithEnvPrefix("MYAPP_") maps MYAPP_SERVER_PORT to SERVER.PORT.
func WithEnvPrefix(prefix string) LoadOption {
	return func(o *loadOptions) {
		o.envPrefix = prefix
	}
}

// WithValidation runs Config.Validate after loading defaults.
func WithValidation() LoadOption {
	return func(o *loadOptions) {
		o.validate = true
	}
}

// WithEnvFile changes the dotenv file name loaded from each config path.
func WithEnvFile(name string) LoadOption {
	return func(o *loadOptions) {
		if name != "" {
			o.fileName = name
		}
	}
}

// EnvOnly skips dotenv files and loads only process environment variables.
func EnvOnly() LoadOption {
	return func(o *loadOptions) {
		o.envOnly = true
	}
}

// Load reads .env files from the supplied paths, then overlays process
// environment variables. Environment variables use underscores for nesting:
// SERVER_PORT becomes SERVER.PORT.
func Load(paths ...string) (Config, error) {
	return LoadWithOptions(paths)
}

// LoadWithOptions is the configurable version of Load.
func LoadWithOptions(paths []string, opts ...LoadOption) (Config, error) {
	var cfg Config
	options := defaultLoadOptions(paths)
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	k := koanf.New(".")

	if !options.envOnly {
		for _, configPath := range options.paths {
			if err := loadDotenv(k, configPath, options.fileName); err != nil {
				return cfg, err
			}
		}
	}

	if err := k.Load(env.Provider(options.envPrefix, ".", envKeyMapper(options.envPrefix)), nil); err != nil {
		return cfg, fmt.Errorf("environment load failed: %w", err)
	}

	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		Tag: "mapstructure",
		DecoderConfig: &mapstructure.DecoderConfig{
			Result:           &cfg,
			Squash:           true,
			WeaklyTypedInput: true,
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			),
		},
	}); err != nil {
		return cfg, fmt.Errorf("config parse failed: %w", err)
	}

	applyDefaults(&cfg, k)

	if err := RequireEnv(options.required...); err != nil {
		return cfg, err
	}
	if options.validate {
		if err := cfg.Validate(); err != nil {
			return cfg, err
		}
	}

	return cfg, nil
}

// MustLoad panics when configuration cannot be loaded.
func MustLoad(paths ...string) Config {
	cfg, err := Load(paths...)
	if err != nil {
		panic(err)
	}
	return cfg
}

func loadDotenv(k *koanf.Koanf, configPath string, fileName string) error {
	if configPath == "" {
		configPath = "."
	}

	path := configPath
	if info, err := os.Stat(configPath); err == nil && info.IsDir() {
		path = filepath.Join(configPath, fileName)
	} else if errors.Is(err, os.ErrNotExist) && filepath.Ext(configPath) == "" {
		path = filepath.Join(configPath, fileName)
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("config stat failed for %s: %w", path, err)
	}

	if err := k.Load(file.Provider(path), dotenv.ParserEnv("", ".", envKeyMapper(""))); err != nil {
		return fmt.Errorf("dotenv load failed for %s: %w", path, err)
	}

	return nil
}

func envKeyMapper(prefix string) func(string) string {
	return func(key string) string {
		key = strings.TrimPrefix(key, prefix)
		key = strings.ToUpper(key)
		if i := strings.Index(key, "_"); i >= 0 {
			return key[:i] + "." + key[i+1:]
		}
		return key
	}
}

func applyDefaults(cfg *Config, k *koanf.Koanf) {
	if cfg.App.Name == "" {
		cfg.App.Name = "go-service"
	}
	if cfg.App.Env == "" {
		cfg.App.Env = cfg.Server.Env
	}
	if cfg.App.Env == "" {
		cfg.App.Env = "production"
	}

	if cfg.Server.Env == "" {
		cfg.Server.Env = cfg.App.Env
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 10 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 120 * time.Second
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 15 * time.Second
	}

	if !isZeroPostgres(cfg.Database) && isZeroPostgres(cfg.Postgres) {
		cfg.Postgres = cfg.Database
	}
	if isZeroPostgres(cfg.Database) {
		cfg.Database = cfg.Postgres
	}

	applyPostgresDefaults(&cfg.Postgres)
	applyPostgresDefaults(&cfg.Database)
	applyRedisDefaults(&cfg.Redis)
	applyAuthDefaults(&cfg.Auth)
	applyTelemetryDefaults(cfg, k)
}

func applyPostgresDefaults(cfg *PostgresConfig) {
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "require"
	}
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = 25
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 25
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = time.Hour
	}
	if cfg.ConnMaxIdleTime == 0 {
		cfg.ConnMaxIdleTime = 5 * time.Minute
	}
	if cfg.PingTimeout == 0 {
		cfg.PingTimeout = 5 * time.Second
	}
}

func applyRedisDefaults(cfg *RedisConfig) {
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

func applyAuthDefaults(cfg *AuthConfig) {
	if cfg.AccessTTL == 0 {
		cfg.AccessTTL = 15 * time.Minute
	}
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = 7 * 24 * time.Hour
	}
	if cfg.Leeway == 0 {
		cfg.Leeway = 30 * time.Second
	}
}

func applyTelemetryDefaults(cfg *Config, k *koanf.Koanf) {
	if cfg.Telemetry.ServiceName == "" {
		cfg.Telemetry.ServiceName = cfg.App.Name
	}
	if cfg.Telemetry.Environment == "" {
		cfg.Telemetry.Environment = cfg.App.Env
	}
	if cfg.Telemetry.Version == "" {
		cfg.Telemetry.Version = cfg.App.Version
	}
	if cfg.Telemetry.PrometheusPath == "" {
		cfg.Telemetry.PrometheusPath = "/metrics"
	}
	if cfg.Telemetry.SampleRatio == 0 {
		cfg.Telemetry.SampleRatio = 1
	}

	telemetryEnabledSet := k != nil && k.Exists("TELEMETRY.ENABLED")
	if !telemetryEnabledSet {
		cfg.Telemetry.Enabled = true
	}
	if k == nil || !k.Exists("TELEMETRY.TRACES_ENABLED") {
		cfg.Telemetry.TracesEnabled = cfg.Telemetry.Enabled
	}
	if k == nil || !k.Exists("TELEMETRY.METRICS_ENABLED") {
		cfg.Telemetry.MetricsEnabled = cfg.Telemetry.Enabled
	}
	if k == nil || !k.Exists("TELEMETRY.PROMETHEUS_ENABLED") {
		cfg.Telemetry.PrometheusEnabled = cfg.Telemetry.Enabled
	}
}

func isZeroPostgres(cfg PostgresConfig) bool {
	return !cfg.Enabled &&
		cfg.Host == "" &&
		cfg.User == "" &&
		cfg.Password == "" &&
		cfg.Database == "" &&
		cfg.Port == 0 &&
		cfg.SSLMode == "" &&
		cfg.MaxOpenConns == 0 &&
		cfg.MaxIdleConns == 0 &&
		cfg.ConnMaxLifetime == 0 &&
		cfg.ConnMaxIdleTime == 0 &&
		cfg.PingTimeout == 0
}

// RequireEnv reports a single error containing all missing environment names.
func RequireEnv(keys ...string) error {
	var missing []string
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if _, ok := os.LookupEnv(key); !ok {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return nil
}

// Validate checks cross-field configuration that cannot be expressed by defaults.
func (cfg Config) Validate() error {
	var errs []error

	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		errs = append(errs, fmt.Errorf("server port must be between 1 and 65535"))
	}
	if cfg.Postgres.Enabled {
		if cfg.Postgres.Host == "" {
			errs = append(errs, fmt.Errorf("postgres host is required when postgres is enabled"))
		}
		if cfg.Postgres.User == "" {
			errs = append(errs, fmt.Errorf("postgres user is required when postgres is enabled"))
		}
		if cfg.Postgres.Database == "" {
			errs = append(errs, fmt.Errorf("postgres database is required when postgres is enabled"))
		}
	}
	if cfg.Auth.JWTSecret != "" && len(cfg.Auth.JWTSecret) < 32 {
		errs = append(errs, fmt.Errorf("auth jwt secret should be at least 32 bytes"))
	}
	if cfg.Telemetry.SampleRatio < 0 || cfg.Telemetry.SampleRatio > 1 {
		errs = append(errs, fmt.Errorf("telemetry sample ratio must be between 0 and 1"))
	}

	return errors.Join(errs...)
}

// Addr returns the host:port pair for server startup.
func (cfg ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

// Addr returns the host:port pair for Redis.
func (cfg RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

// IsProduction reports whether the app is running in production mode.
func (cfg Config) IsProduction() bool {
	return strings.EqualFold(cfg.App.Env, "production") || strings.EqualFold(cfg.Server.Env, "production")
}

// PostgresConfig returns the canonical database configuration.
func (cfg Config) PostgresConfig() PostgresConfig {
	if !isZeroPostgres(cfg.Postgres) {
		return cfg.Postgres
	}
	return cfg.Database
}
