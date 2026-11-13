package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

const (
	sessionCookieName = "runcoil_session"
	maxBodyBytes      = 1 << 20
)

var (
	errNotSignedIn      = apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in")
	errPermissionDenied = apierr.PermissionDenied(apierr.CodePermissionDenied, "permission denied")
)

type callerKey struct{}

type caller struct {
	user  domain.User
	token string
}

func callerFrom(ctx context.Context) (caller, bool) {
	c, ok := ctx.Value(callerKey{}).(caller)
	return c, ok
}

func (s *Service) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}

			if v == http.ErrAbortHandler { //nolint:errorlint // recover returns the value as it was panicked.
				panic(v)
			}

			s.logger.ErrorContext(r.Context(), "request panicked",
				slog.Any("panic", v),
				slog.String("stack", string(debug.Stack())),
			)
			writeError(w, http.StatusInternalServerError, gen.ErrorCodeInternal, internalMessage, "")
		}()

		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (s *Service) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		s.logger.DebugContext(r.Context(), "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// loadSession puts the caller of a live session cookie in the context. A request without
// one goes on without a caller, and the permission check answers it.
func (s *Service) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		user, err := s.accounts.Authenticate(r.Context(), cookie.Value)

		var apiErr *apierr.Error
		if errors.As(err, &apiErr) && apiErr.Kind == apierr.KindUnauthenticated {
			next.ServeHTTP(w, r)
			return
		}

		if err != nil {
			s.handleError(w, r, err)
			return
		}

		ctx := context.WithValue(r.Context(), callerKey{}, caller{user: user, token: cookie.Value})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func checkOrigin(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusForbidden, gen.ErrorCodePermissionDenied, "cross-origin request refused", "")
	}))

	return protection.Handler(next)
}

func (s *Service) validateRequest(spec *openapi3.T) func(http.Handler) http.Handler {
	return nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		ErrorHandlerWithOpts: s.handleValidationError,
		DoNotValidateServers: true,
		Prefix:               apiPrefix,
	})
}

func (s *Service) handleValidationError(_ context.Context, err error, w http.ResponseWriter, _ *http.Request, _ nethttpmiddleware.ErrorHandlerOpts) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, gen.ErrorCodePayloadTooLarge, "request body is too large", "")
		return
	}

	if errors.Is(err, routers.ErrPathNotFound) {
		writeError(w, http.StatusNotFound, gen.ErrorCodeNotFound, "no such operation", "")
		return
	}

	if errors.Is(err, routers.ErrMethodNotAllowed) {
		writeError(w, http.StatusMethodNotAllowed, gen.ErrorCodeInvalidRequest, "method not allowed", "")
		return
	}

	var violation *jsonschema.ValidationError
	if errors.As(err, &violation) {
		code, text, field := describeViolation(violation)
		writeError(w, http.StatusBadRequest, code, text, field)

		return
	}

	var requestErr *openapi3filter.RequestError
	if errors.As(err, &requestErr) && requestErr.Parameter != nil {
		writeError(w, http.StatusBadRequest, gen.ErrorCodeInvalidField, requestErr.Reason, requestErr.Parameter.Name)
		return
	}

	writeError(w, http.StatusBadRequest, gen.ErrorCodeInvalidRequest, "request is not valid", "")
}

func describeViolation(violation *jsonschema.ValidationError) (gen.ErrorCode, string, string) {
	for len(violation.Causes) > 0 {
		violation = violation.Causes[0]
	}

	location := violation.InstanceLocation

	switch k := violation.ErrorKind.(type) {
	case *kind.Required:
		field := strings.Join(append(slices.Clone(location), k.Missing[0]), ".")
		return gen.ErrorCodeMissingField, field + " is required", field
	case *kind.AdditionalProperties:
		field := strings.Join(append(slices.Clone(location), k.Properties[0]), ".")
		return gen.ErrorCodeInvalidField, field + " is not allowed", field
	default:
		field := strings.Join(location, ".")
		return gen.ErrorCodeInvalidField, field + " is not valid: " + k.LocalizedString(message.NewPrinter(language.English)), field
	}
}

func (s *Service) checkPermission(f gen.StrictHandlerFunc, operationID string) gen.StrictHandlerFunc {
	permission, ok := s.permissions[operationID]
	if !ok {
		return func(context.Context, http.ResponseWriter, *http.Request, any) (any, error) {
			return nil, fmt.Errorf("operation %s has no permission in the spec", operationID)
		}
	}

	if permission == permissionPublic {
		return f
	}

	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		c, ok := callerFrom(ctx)
		if !ok {
			return nil, errNotSignedIn
		}

		if permission != permissionSelf {
			if err := s.authorize(ctx, c.user, gen.Permission(permission), r.PathValue("projectId")); err != nil {
				return nil, err
			}
		}

		return f(ctx, w, r, request)
	}
}

func (s *Service) authorize(ctx context.Context, user domain.User, permission gen.Permission, projectID string) error {
	if user.IsServerAdmin {
		return nil
	}

	id, err := uuid.Parse(projectID)
	if err != nil {
		return errPermissionDenied
	}

	role, ok, err := s.accounts.Role(ctx, user, id)
	if err != nil {
		return err
	}

	if !ok || !roleHas(role, permission) {
		return errPermissionDenied
	}

	return nil
}

func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}

	return h
}
