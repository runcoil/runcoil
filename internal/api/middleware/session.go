package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/runcoil/runcoil/internal/apperr"
	"github.com/runcoil/runcoil/internal/domain"
)

// Caller is the signed-in user of a request and the session token it came with.
type Caller struct {
	User  domain.User
	Token string
}

type callerKey struct{}

func withCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFromContext returns the caller that LoadSession put in ctx. It reports false
// when the request has no live session.
func CallerFromContext(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok
}

// Authenticator returns the user of a session token.
type Authenticator interface {
	AuthenticateSession(ctx context.Context, token string) (domain.User, error)
}

// LoadSession puts the caller of a live session cookie in the context. A request without
// one goes on without a caller, and the permission check answers it. Any other failure
// goes to handleError.
func LoadSession(cookieName string, authenticator Authenticator, handleError func(http.ResponseWriter, *http.Request, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			user, err := authenticator.AuthenticateSession(r.Context(), cookie.Value)

			var appErr *apperr.Error
			if errors.As(err, &appErr) && appErr.Kind == apperr.KindUnauthenticated {
				next.ServeHTTP(w, r)
				return
			}

			if err != nil {
				handleError(w, r, err)
				return
			}

			recordUserID(r.Context(), user.ID.String())
			next.ServeHTTP(w, r.WithContext(withCaller(r.Context(), Caller{User: user, Token: cookie.Value})))
		})
	}
}
