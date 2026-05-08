package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/yosi-hq/go-backend-kit/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type GormOption func(*gormOptions)

type gormOptions struct {
	gormConfig      *gorm.Config
	pgOpts          []PostgresOption
	telemetry       bool
	includeSQL      bool
	telemetrySource string
}

func defaultGormOptions() gormOptions {
	return gormOptions{
		gormConfig: &gorm.Config{
			SkipDefaultTransaction: true,
		},
		telemetry:       true,
		telemetrySource: "gorm",
	}
}

// WithGormConfig overrides the gorm configuration used by ConnectGorm.
func WithGormConfig(cfg *gorm.Config) GormOption {
	return func(o *gormOptions) {
		if cfg != nil {
			o.gormConfig = cfg
		}
	}
}

// WithPostgresOptions applies PostgresOption values to the underlying database/sql connection.
func WithPostgresOptions(opts ...PostgresOption) GormOption {
	return func(o *gormOptions) {
		o.pgOpts = append(o.pgOpts, opts...)
	}
}

// WithGormTelemetry toggles query spans and metrics emitted by GORM callbacks.
func WithGormTelemetry(enabled bool) GormOption {
	return func(o *gormOptions) {
		o.telemetry = enabled
	}
}

// WithGormSQLInSpans includes the generated SQL statement in spans.
//
// Keep this disabled for high-cardinality or sensitive workloads.
func WithGormSQLInSpans(enabled bool) GormOption {
	return func(o *gormOptions) {
		o.includeSQL = enabled
	}
}

// ConnectGorm opens a gorm.DB backed by an instrumented database/sql connection.
//
// The returned gorm.DB can be closed via:
//
//	sqlDB, _ := gdb.DB()
//	_ = sqlDB.Close()
func ConnectGorm(ctx context.Context, cfg config.PostgresConfig, opts ...GormOption) (*gorm.DB, error) {
	o := defaultGormOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	sqlDB, err := ConnectPostgres(ctx, cfg, o.pgOpts...)
	if err != nil {
		return nil, err
	}

	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), o.gormConfig)
	if err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("gorm open failed: %w", err)
	}

	if o.telemetry {
		if err := gdb.Use(NewGormTelemetryPlugin(GormTelemetryConfig{
			ServiceName: o.telemetrySource,
			DBName:      cfg.Database,
			IncludeSQL:  o.includeSQL,
		})); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("gorm telemetry setup failed: %w", err)
		}
	}

	return gdb, nil
}

// CloseGorm closes the underlying sql.DB of a gorm.DB.
func CloseGorm(gdb *gorm.DB) error {
	if gdb == nil {
		return nil
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// DBFromGorm returns the underlying *sql.DB for pool configuration or low-level access.
func DBFromGorm(gdb *gorm.DB) (*sql.DB, error) {
	if gdb == nil {
		return nil, nil
	}
	return gdb.DB()
}
