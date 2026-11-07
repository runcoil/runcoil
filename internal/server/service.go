package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
)

const (
	readHeaderTimeout = 10 * time.Second
	readyTimeout      = 2 * time.Second
	shutdownTimeout   = 15 * time.Second

	// Time for load balancers to see /readyz fail before the listener closes.
	defaultShutdownDelay = 2 * time.Second
)

// Config is the HTTP server configuration.
type Config struct {
	Addr string
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Addr, "http.addr", ":7070", "Address to listen on, for every API, health and metrics.")
}

// Validate returns every invalid value at once.
func (c *Config) Validate() error {
	_, port, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return fmt.Errorf("http.addr must be host:port: %w", err)
	}

	if n, err := strconv.Atoi(port); err != nil || n < 0 || n > 65535 {
		return errors.New("http.addr port must be a number between 0 and 65535")
	}

	return nil
}

// Pinger checks that the database answers.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Service serves health, metrics and, later, the APIs on one port.
type Service struct {
	cfg           Config
	logger        *slog.Logger
	db            Pinger
	server        *http.Server
	shutdownDelay time.Duration

	shuttingDown atomic.Bool
	stopOnce     sync.Once
	stopped      chan struct{}
	stopErr      error
}

// NewService builds the HTTP service. It listens when Run is called.
func NewService(cfg Config, logger *slog.Logger, db Pinger) *Service {
	s := &Service{
		cfg:           cfg,
		logger:        logger,
		db:            db,
		shutdownDelay: defaultShutdownDelay,
		stopped:       make(chan struct{}),
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", s.livez)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.Handle("GET /metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	s.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		Protocols:         &protocols,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return s
}

// Run listens and serves until Stop. It returns nil after a clean shutdown.
func (s *Service) Run(ctx context.Context) error {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.logger.InfoContext(ctx, "http server is listening", slog.String("addr", listener.Addr().String()))

	if err := s.server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}

	// Serve returns as soon as Shutdown starts; the requests in flight finish first.
	<-s.stopped

	return s.stopErr
}

// Stop turns /readyz to 503, waits for load balancers to notice, and shuts the
// server down gracefully.
func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		s.shuttingDown.Store(true)
		time.Sleep(s.shutdownDelay)

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.server.Shutdown(ctx); err != nil {
			s.stopErr = fmt.Errorf("shutdown: %w", err)
		}
		close(s.stopped)
	})
}

func (s *Service) livez(w http.ResponseWriter, _ *http.Request) {
	writeText(w, http.StatusOK, "ok")
}

func (s *Service) readyz(w http.ResponseWriter, r *http.Request) {
	if s.shuttingDown.Load() {
		writeText(w, http.StatusServiceUnavailable, "shutting down")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		s.logger.WarnContext(ctx, "database ping failed", slog.Any("error", err))
		writeText(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	writeText(w, http.StatusOK, "ok")
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}
