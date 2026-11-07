package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/spf13/pflag"
)

const (
	readyTimeout    = 2 * time.Second
	shutdownTimeout = 10 * time.Second
)

// Config is the engine configuration.
type Config struct {
	Port int
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.Port, "engine.port", 7070, "Port of the APIs and the health checks.")
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}

	return nil
}

// Pinger checks that the database answers.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Service serves the health checks, and later the APIs, on one port.
type Service struct {
	cfg    Config
	logger *slog.Logger
	db     Pinger

	stopped  chan struct{}
	stopOnce sync.Once
}

// NewService creates the engine service.
func NewService(cfg Config, logger *slog.Logger, db Pinger) *Service {
	return &Service{
		cfg:     cfg,
		logger:  logger,
		db:      db,
		stopped: make(chan struct{}),
	}
}

// Run serves until Stop is called or ctx ends, then shuts the HTTP server down.
func (s *Service) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	return s.serve(ctx, listener)
}

func (s *Service) serve(ctx context.Context, listener net.Listener) error {
	srv := s.newHTTPServer()

	served := make(chan error, 1)
	go func() {
		served <- srv.Serve(listener)
	}()

	s.logger.InfoContext(ctx, "engine listening", slog.String("addr", listener.Addr().String()))

	select {
	case <-s.stopped:
	case <-ctx.Done():
	case err := <-served:
		return fmt.Errorf("serve http: %w", err)
	}

	s.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http: %w", errors.Join(err, srv.Close()))
	}

	return nil
}

// Stop starts a graceful shutdown. It is safe to call more than once, and before Run.
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

func (s *Service) newHTTPServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", s.livez)
	mux.HandleFunc("GET /readyz", s.readyz)

	// HTTP/1.1 for plain requests, and HTTP/2 without TLS for the Connect streams.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	return &http.Server{
		Handler:           mux,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func (s *Service) livez(w http.ResponseWriter, _ *http.Request) {
	writeHealth(w, http.StatusOK, "ok")
}

func (s *Service) readyz(w http.ResponseWriter, r *http.Request) {
	if isStopping(s.stopped) {
		writeHealth(w, http.StatusServiceUnavailable, "shutting down")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()

	if err := s.db.Ping(ctx); err != nil {
		s.logger.WarnContext(ctx, "database ping failed", slog.Any("error", err))
		writeHealth(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	writeHealth(w, http.StatusOK, "ok")
}

func isStopping(stopped <-chan struct{}) bool {
	select {
	case <-stopped:
		return true
	default:
		return false
	}
}

func writeHealth(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
