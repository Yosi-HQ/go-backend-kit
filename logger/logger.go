package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	DebugContext(ctx context.Context, msg string, args ...any)
	InfoContext(ctx context.Context, msg string, args ...any)
	WarnContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
	With(args ...any) Logger
	Unwrap() *slog.Logger
}

type Option func(*options)

type options struct {
	output io.Writer
	level  slog.Level
	text   bool
	attrs  []any
}

type SlogLogger struct {
	log *slog.Logger
}

func New(env string, opts ...Option) Logger {
	o := options{
		output: os.Stdout,
		level:  levelForEnv(env),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	handlerOptions := &slog.HandlerOptions{Level: o.level}
	var handler slog.Handler
	if o.text {
		handler = slog.NewTextHandler(o.output, handlerOptions)
	} else {
		handler = slog.NewJSONHandler(o.output, handlerOptions)
	}

	log := slog.New(handler)
	if len(o.attrs) > 0 {
		log = log.With(o.attrs...)
	}

	return &SlogLogger{log: log}
}

func NewWithSlog(log *slog.Logger) Logger {
	if log == nil {
		return New("production")
	}
	return &SlogLogger{log: log}
}

func WithOutput(w io.Writer) Option {
	return func(o *options) {
		if w != nil {
			o.output = w
		}
	}
}

func WithLevel(level slog.Level) Option {
	return func(o *options) {
		o.level = level
	}
}

func WithLevelString(level string) Option {
	return func(o *options) {
		o.level = ParseLevel(level, o.level)
	}
}

func WithTextHandler() Option {
	return func(o *options) {
		o.text = true
	}
}

func WithAttrs(args ...any) Option {
	return func(o *options) {
		o.attrs = append(o.attrs, args...)
	}
}

func ParseLevel(level string, fallback slog.Level) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return fallback
	}
}

func levelForEnv(env string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "dev", "development", "local", "test", "testing":
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

func (l *SlogLogger) Debug(msg string, args ...any) {
	l.log.Debug(msg, args...)
}

func (l *SlogLogger) Info(msg string, args ...any) {
	l.log.Info(msg, args...)
}

func (l *SlogLogger) Warn(msg string, args ...any) {
	l.log.Warn(msg, args...)
}

func (l *SlogLogger) Error(msg string, args ...any) {
	l.log.Error(msg, args...)
}

func (l *SlogLogger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.log.DebugContext(ctx, msg, args...)
}

func (l *SlogLogger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.log.InfoContext(ctx, msg, args...)
}

func (l *SlogLogger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.log.WarnContext(ctx, msg, args...)
}

func (l *SlogLogger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.log.ErrorContext(ctx, msg, args...)
}

func (l *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{log: l.log.With(args...)}
}

func (l *SlogLogger) Unwrap() *slog.Logger {
	return l.log
}
