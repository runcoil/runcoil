package api

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
	"uuid"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/internal/service/account"
	"github.com/runcoil/runcoil/openapi"
	"github.com/runcoil/runcoil/pkg/healthutil"
)

const (
	readyTimeout    = 2 * time.Second
	shutdownTimeout = 10 * time.Second

	apiPrefix = "/api/v1"
)

// Config is the configuration of the API server.
type Config struct {
	Port int
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.Port, "api.port", 8080, "Port of the API and the health checks.")
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}

	return nil
}

// AccountService signs users in and out and checks their roles.
type AccountService interface {
	Login(ctx context.Context, login, password string) (account.LoginResult, error)
	Authenticate(ctx context.Context, token string) (domain.User, error)
	Logout(ctx context.Context, token string) error
	ChangePassword(ctx context.Context, user domain.User, token, currentPassword, newPassword string) error
	ProjectRoles(ctx context.Context, user domain.User) ([]domain.ProjectRole, error)
	Role(ctx context.Context, user domain.User, projectID uuid.UUID) (domain.Role, bool, error)
}

// Service serves the dashboard API and the health checks of the process.
type Service struct {
	cfg      Config
	logger   *slog.Logger
	checks   *healthutil.Checks
	accounts AccountService

	spec        *openapi3.T
	permissions map[string]string

	stopped  chan struct{}
	stopOnce sync.Once
}

// New creates the API service. Its /readyz runs checks.
func New(cfg Config, logger *slog.Logger, checks *healthutil.Checks, accounts AccountService) (*Service, error) {
	spec, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}

	permissions, err := operationPermissions(spec)
	if err != nil {
		return nil, fmt.Errorf("read permissions: %w", err)
	}

	return &Service{
		cfg:         cfg,
		logger:      logger,
		checks:      checks,
		accounts:    accounts,
		spec:        spec,
		permissions: permissions,
		stopped:     make(chan struct{}),
	}, nil
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

	s.logger.InfoContext(ctx, "api server listening", slog.String("addr", listener.Addr().String()))

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
	mux.Handle("/api/", s.apiHandler())

	return &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

func (s *Service) apiHandler() http.Handler {
	strict := gen.NewStrictHandlerWithOptions(
		&handler{authHandler: &authHandler{accounts: s.accounts}},
		[]gen.StrictMiddlewareFunc{s.checkPermission},
		gen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  s.handleRequestError,
			ResponseErrorHandlerFunc: s.handleError,
		},
	)

	routes := gen.HandlerWithOptions(strict, gen.StdHTTPServerOptions{
		BaseURL:          apiPrefix,
		BaseRouter:       http.NewServeMux(),
		ErrorHandlerFunc: s.handleRequestError,
	})

	return chain(routes,
		s.recoverPanic,
		s.logRequest,
		limitBody,
		s.loadSession,
		checkOrigin,
		s.validateRequest(s.spec),
	)
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

	if err := s.checks.Ready(ctx); err != nil {
		s.logger.WarnContext(ctx, "readiness check failed", slog.Any("error", err))
		writeHealth(w, http.StatusServiceUnavailable, "not ready")
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
