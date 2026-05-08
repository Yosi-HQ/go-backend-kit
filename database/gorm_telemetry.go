package database

import (
	"fmt"
	"time"

	"github.com/yosi-hq/go-backend-kit/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const (
	gormTelemetryPluginName = "go-backend-kit:gorm-telemetry"
	gormStartKey            = "go-backend-kit:gorm-start"
	gormSpanKey             = "go-backend-kit:gorm-span"
)

type GormTelemetryConfig struct {
	ServiceName string
	DBName      string
	IncludeSQL  bool
}

type GormTelemetryPlugin struct {
	cfg    GormTelemetryConfig
	tracer trace.Tracer
}

func NewGormTelemetryPlugin(cfg GormTelemetryConfig) *GormTelemetryPlugin {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "go-backend-kit/database"
	}

	return &GormTelemetryPlugin{
		cfg:    cfg,
		tracer: otel.Tracer(cfg.ServiceName),
	}
}

func (p *GormTelemetryPlugin) Name() string {
	return gormTelemetryPluginName
}

func (p *GormTelemetryPlugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Create().Before("gorm:create").Register(p.callbackName("before", "create"), p.before("create")); err != nil {
		return err
	}
	if err := db.Callback().Create().After("gorm:create").Register(p.callbackName("after", "create"), p.after("create")); err != nil {
		return err
	}
	if err := db.Callback().Query().Before("gorm:query").Register(p.callbackName("before", "query"), p.before("query")); err != nil {
		return err
	}
	if err := db.Callback().Query().After("gorm:query").Register(p.callbackName("after", "query"), p.after("query")); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register(p.callbackName("before", "update"), p.before("update")); err != nil {
		return err
	}
	if err := db.Callback().Update().After("gorm:update").Register(p.callbackName("after", "update"), p.after("update")); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register(p.callbackName("before", "delete"), p.before("delete")); err != nil {
		return err
	}
	if err := db.Callback().Delete().After("gorm:delete").Register(p.callbackName("after", "delete"), p.after("delete")); err != nil {
		return err
	}
	if err := db.Callback().Raw().Before("gorm:raw").Register(p.callbackName("before", "raw"), p.before("raw")); err != nil {
		return err
	}
	if err := db.Callback().Raw().After("gorm:raw").Register(p.callbackName("after", "raw"), p.after("raw")); err != nil {
		return err
	}
	if err := db.Callback().Row().Before("gorm:row").Register(p.callbackName("before", "row"), p.before("row")); err != nil {
		return err
	}
	return db.Callback().Row().After("gorm:row").Register(p.callbackName("after", "row"), p.after("row"))
}

func (p *GormTelemetryPlugin) callbackName(phase string, operation string) string {
	return fmt.Sprintf("%s:%s:%s", gormTelemetryPluginName, phase, operation)
}

func (p *GormTelemetryPlugin) before(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		table := tx.Statement.Table
		if table == "" && tx.Statement.Schema != nil {
			table = tx.Statement.Schema.Table
		}

		spanName := "db." + operation
		if table != "" {
			spanName = spanName + " " + table
		}

		ctx, span := p.tracer.Start(tx.Statement.Context, spanName)
		span.SetAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", operation),
			attribute.String("db.name", p.cfg.DBName),
			attribute.String("db.sql.table", table),
		)

		tx.Statement.Context = ctx
		tx.InstanceSet(gormStartKey, time.Now())
		tx.InstanceSet(gormSpanKey, span)
	}
}

func (p *GormTelemetryPlugin) after(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		start, _ := tx.InstanceGet(gormStartKey)
		startTime, _ := start.(time.Time)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		duration := time.Since(startTime)

		table := tx.Statement.Table
		if table == "" && tx.Statement.Schema != nil {
			table = tx.Statement.Schema.Table
		}

		status := "ok"
		if tx.Error != nil {
			status = "error"
		}

		if spanValue, ok := tx.InstanceGet(gormSpanKey); ok {
			if span, ok := spanValue.(trace.Span); ok {
				span.SetAttributes(
					attribute.String("db.sql.table", table),
					attribute.Int64("db.rows_affected", tx.RowsAffected),
				)
				if p.cfg.IncludeSQL && tx.Statement.SQL.Len() > 0 {
					span.SetAttributes(attribute.String("db.statement", tx.Statement.SQL.String()))
				}
				if tx.Error != nil {
					span.RecordError(tx.Error)
					span.SetStatus(codes.Error, tx.Error.Error())
				}
				span.End()
			}
		}

		telemetry.ObserveDBQuery(operation, table, status, duration)
	}
}
