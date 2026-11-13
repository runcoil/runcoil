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
	identity domain.Identity
	err      error
	got      *domain.Credential
}

func (f fakeAuthenticator) Authenticate(_ context.Context, credential domain.Credential) (domain.Identity, error) {
	if f.got != nil {
		*f.got = credential
	}

	return f.identity, f.err
}

func TestAuthenticate(t *testing.T) {
	ana := domain.Identity{Type: domain.IdentityUser, UserID: uuid.MustParse("019a5b1e-7c2d-7000-8000-4b1d2c3e4f50")}

	tests := []struct {
		name          string
		cookie        string
		authenticator fakeAuthenticator
		wantIdentity  bool
		wantHandled   bool
	}{
		{name: "no cookie"},
		{name: "empty cookie", cookie: ""},
		{name: "live session", cookie: "token", authenticator: fakeAuthenticator{identity: ana}, wantIdentity: true},
		{name: "ended session", cookie: "token", authenticator: fakeAuthenticator{err: apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in")}},
		{name: "store failure", cookie: "token", authenticator: fakeAuthenticator{err: assert.AnError}, wantHandled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handled error
			var reached bool
			var identity domain.Identity
			var hasIdentity bool
			var credential domain.Credential
			tt.authenticator.got = &credential
			h := Authenticate("session", tt.authenticator, func(_ http.ResponseWriter, _ *http.Request, err error) {
				handled = err
			})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				reached = true
				identity, hasIdentity = IdentityFromContext(r.Context())
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
			assert.Equal(t, tt.wantIdentity, hasIdentity)

			if tt.wantIdentity {
				assert.Equal(t, ana, identity)
				assert.Equal(t, domain.Credential{Type: domain.CredentialSession, Secret: "token"}, credential)
			}
		})
	}
}

func TestAuthenticatePutsTheUserOnTheLogLine(t *testing.T) {
	logger, buf := newTestLogger()
	ana := domain.Identity{UserID: uuid.MustParse("019a5b1e-7c2d-7000-8000-4b1d2c3e4f50")}
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		LogRequest(logger),
		Authenticate("session", fakeAuthenticator{identity: ana}, nil),
	)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: "token"})

	h.ServeHTTP(httptest.NewRecorder(), r)

	assert.Equal(t, "019a5b1e-7c2d-7000-8000-4b1d2c3e4f50", decodeLogLine(t, buf)["user_id"])
}
