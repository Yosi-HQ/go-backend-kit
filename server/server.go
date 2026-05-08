package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/yosi-hq/go-backend-kit/config"
	"github.com/yosi-hq/go-backend-kit/logger"
)

type Option func(*Server)

type Server struct {
	HTTP            *http.Server
	ShutdownTimeout time.Duration
	Logger          logger.Logger
}

func New(cfg config.ServerConfig, handler http.Handler, opts ...Option) *Server {
	if handler == nil {
		handler = http.DefaultServeMux
	}
	if cfg.Host == "" {
		cfg.Host = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 120 * time.Second
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 15 * time.Second
	}

	srv := &Server{
		HTTP: &http.Server{
			Addr:              net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port)),
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
		ShutdownTimeout: cfg.ShutdownTimeout,
		Logger:          logger.New(cfg.Env),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(srv)
		}
	}

	return srv
}

func WithHTTPServer(httpServer *http.Server) Option {
	return func(s *Server) {
		if httpServer != nil {
			s.HTTP = httpServer
		}
	}
}

func WithLogger(log logger.Logger) Option {
	return func(s *Server) {
		if log != nil {
			s.Logger = log
		}
	}
}

func WithShutdownTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		if timeout > 0 {
			s.ShutdownTimeout = timeout
		}
	}
}

func (s *Server) Run(ctx context.Context) error {
	if s == nil || s.HTTP == nil {
		return fmt.Errorf("http server is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	errCh := make(chan error, 1)
	go func() {
		if s.Logger != nil {
			s.Logger.Info("http server starting", "addr", s.HTTP.Addr)
		}
		err := s.HTTP.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.HTTP == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := s.ShutdownTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if s.Logger != nil {
		s.Logger.Info("http server shutting down", "timeout", timeout.String())
	}

	if err := s.HTTP.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http server shutdown failed: %w", err)
	}
	return nil
}

func ContextWithSignals(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}
