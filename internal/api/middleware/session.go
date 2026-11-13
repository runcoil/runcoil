package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

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

			var apiErr *apierr.Error
			if errors.As(err, &apiErr) && apiErr.Kind == apierr.KindUnauthenticated {
				next.ServeHTTP(w, r)
				return
			}

			if err != nil {
				handleError(w, r, err)
				return
			}

			recordUserID(r.Context(), user.ID.String())

			ctx := withCaller(r.Context(), Caller{User: user, Token: cookie.Value})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
