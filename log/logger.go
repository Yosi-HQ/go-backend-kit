package log

import (
	"log/slog"
	"os"
)

type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)

	With(args ...any) Logger
}

type SlogLogger struct {
	log *slog.Logger
}

func New(env string) Logger {
	var level slog.Level

	switch env {
	case "production":
		level = slog.LevelInfo
	default:
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	return &SlogLogger{
		log: slog.New(handler),
	}
}

func (l *SlogLogger) Info(msg string, args ...any) {
	l.log.Info(msg, args...)
}

func (l *SlogLogger) Error(msg string, args ...any) {
	l.log.Error(msg, args...)
}

func (l *SlogLogger) Warn(msg string, args ...any) {
	l.log.Warn(msg, args...)
}

func (l *SlogLogger) Debug(msg string, args ...any) {
	l.log.Debug(msg, args...)
}

func (l *SlogLogger) With(args ...any) Logger {
	return &SlogLogger{
		log: l.log.With(args...),
	}
}
