package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

type fakeAuthenticator struct {
	user domain.User
	err  error
}

func (f fakeAuthenticator) AuthenticateSession(context.Context, string) (domain.User, error) {
	return f.user, f.err
}

func TestLoadSession(t *testing.T) {
	ana := domain.User{ID: uuid.MustParse("019a5b1e-7c2d-7000-8000-4b1d2c3e4f50"), Username: "ana"}

	tests := []struct {
		name          string
		cookie        string
		authenticator fakeAuthenticator
		wantCaller    bool
		wantHandled   bool
	}{
		{name: "no cookie"},
		{name: "empty cookie", cookie: ""},
		{name: "live session", cookie: "token", authenticator: fakeAuthenticator{user: ana}, wantCaller: true},
		{name: "ended session", cookie: "token", authenticator: fakeAuthenticator{err: apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in")}},
		{name: "store failure", cookie: "token", authenticator: fakeAuthenticator{err: assert.AnError}, wantHandled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handled error
			var reached bool
			var caller Caller
			var hasCaller bool
			h := LoadSession("session", tt.authenticator, func(_ http.ResponseWriter, _ *http.Request, err error) {
				handled = err
			})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				reached = true
				caller, hasCaller = CallerFromContext(r.Context())
			}))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: "session", Value: tt.cookie})
			}

			h.ServeHTTP(httptest.NewRecorder(), r)

			if tt.wantHandled {
				require.ErrorIs(t, handled, assert.AnError)
				assert.False(t, reached)

				return
			}

			assert.NoError(t, handled)
			assert.True(t, reached)
			assert.Equal(t, tt.wantCaller, hasCaller)

			if tt.wantCaller {
				assert.Equal(t, Caller{User: ana, Token: "token"}, caller)
			}
		})
	}
}

func TestLoadSessionPutsTheUserOnTheLogLine(t *testing.T) {
	logger, buf := newTestLogger()
	ana := domain.User{ID: uuid.MustParse("019a5b1e-7c2d-7000-8000-4b1d2c3e4f50")}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		LogRequest(logger),
		LoadSession("session", fakeAuthenticator{user: ana}, nil),
	)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: "token"})

	h.ServeHTTP(httptest.NewRecorder(), r)

	assert.Equal(t, "019a5b1e-7c2d-7000-8000-4b1d2c3e4f50", decodeLogLine(t, buf)["user_id"])
}
