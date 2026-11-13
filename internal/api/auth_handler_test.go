package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/internal/service/account"
	"github.com/runcoil/runcoil/pkg/healthutil"
)

const testToken = "token-1"

var testUser = domain.User{ID: uuid.MustParse("019a5b1e-7c2d-7000-8000-4b1d2c3e4f50"), Username: "ana"}

type fakeAccounts struct {
	user  domain.User
	roles []domain.ProjectRole
	role  domain.Role

	loginErr    error
	changeErr   error
	loggedOut   string
	newPassword string
}

func (f *fakeAccounts) Login(_ context.Context, login, password string) (account.LoginResult, error) {
	if f.loginErr != nil {
		return account.LoginResult{}, f.loginErr
	}

	return account.LoginResult{
		Token:                   testToken,
		ExpiresAt:               time.Date(2026, 12, 13, 0, 0, 0, 0, time.UTC),
		User:                    f.user,
		PasswordChangeSuggested: login == "admin" && password == "admin",
	}, nil
}

func (f *fakeAccounts) Authenticate(_ context.Context, token string) (domain.User, error) {
	if token != testToken {
		return domain.User{}, apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in")
	}

	return f.user, nil
}

func (f *fakeAccounts) Logout(_ context.Context, token string) error {
	f.loggedOut = token
	return nil
}

func (f *fakeAccounts) ChangePassword(_ context.Context, _ domain.User, _, _, newPassword string) error {
	f.newPassword = newPassword
	return f.changeErr
}

func (f *fakeAccounts) ProjectRoles(context.Context, domain.User) ([]domain.ProjectRole, error) {
	return f.roles, nil
}

func (f *fakeAccounts) Role(_ context.Context, _ domain.User, _ uuid.UUID) (domain.Role, bool, error) {
	return f.role, f.role != "", nil
}

func newAPIServer(t *testing.T, accounts *fakeAccounts) http.Handler {
	t.Helper()

	s, err := New(Config{Port: 8080}, slog.New(slog.DiscardHandler), healthutil.New(), accounts)
	require.NoError(t, err)

	return s.newHTTPServer().Handler
}

type call struct {
	method string
	path   string
	body   string
	token  string
	origin string
}

func do(t *testing.T, h http.Handler, c call) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	if c.body != "" {
		r.Header.Set("Content-Type", "application/json")
	}

	if c.token != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: c.token})
	}

	if c.origin != "" {
		r.Header.Set("Origin", c.origin)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	return rec
}

type errorBody struct {
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	} `json:"errors"`
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) (code, field string) {
	t.Helper()

	var body errorBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	require.Len(t, body.Errors, 1)

	return body.Errors[0].Code, body.Errors[0].Field
}

func TestLogin(t *testing.T) {
	accounts := &fakeAccounts{user: testUser}
	h := newAPIServer(t, accounts)

	rec := do(t, h, call{method: http.MethodPost, path: "/api/v1/auth/login", body: `{"login":"admin","password":"admin"}`})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"user":{"id":"019a5b1e-7c2d-7000-8000-4b1d2c3e4f50","username":"ana","isServerAdmin":false},"passwordChangeSuggested":true}`, rec.Body.String())

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionCookieName, cookies[0].Name)
	assert.Equal(t, testToken, cookies[0].Value)
	assert.True(t, cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
	assert.Equal(t, "/", cookies[0].Path)
}

func TestLoginErrors(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		loginErr  error
		wantCode  int
		wantError string
		wantField string
	}{
		{name: "wrong password", body: `{"login":"ana","password":"x"}`, loginErr: apierr.Unauthenticated(apierr.CodeUnauthenticated, "wrong login or password"), wantCode: http.StatusUnauthorized, wantError: "unauthenticated"},
		{name: "missing password", body: `{"login":"ana"}`, wantCode: http.StatusBadRequest, wantError: "missing_field", wantField: "password"},
		{name: "empty login", body: `{"login":"","password":"x"}`, wantCode: http.StatusBadRequest, wantError: "invalid_field", wantField: "login"},
		{name: "unknown field", body: `{"login":"ana","password":"x","extra":1}`, wantCode: http.StatusBadRequest, wantError: "invalid_field", wantField: "extra"},
		{name: "not JSON", body: `{`, wantCode: http.StatusBadRequest, wantError: "invalid_request"},
		{name: "store failure", body: `{"login":"ana","password":"x"}`, loginErr: assert.AnError, wantCode: http.StatusInternalServerError, wantError: "internal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newAPIServer(t, &fakeAccounts{loginErr: tt.loginErr})

			rec := do(t, h, call{method: http.MethodPost, path: "/api/v1/auth/login", body: tt.body})

			assert.Equal(t, tt.wantCode, rec.Code)
			code, field := decodeError(t, rec)
			assert.Equal(t, tt.wantError, code, rec.Body.String())
			assert.Equal(t, tt.wantField, field, rec.Body.String())
		})
	}
}

func TestLogout(t *testing.T) {
	accounts := &fakeAccounts{user: testUser}
	h := newAPIServer(t, accounts)

	rec := do(t, h, call{method: http.MethodPost, path: "/api/v1/auth/logout", token: testToken})

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, testToken, accounts.loggedOut)

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionCookieName, cookies[0].Name)
	assert.Equal(t, -1, cookies[0].MaxAge)
}

func TestGetMe(t *testing.T) {
	projectID := uuid.MustParse("019a5b1e-7c2d-7000-8000-4b1d2c3e4f51")

	tests := []struct {
		name string
		user domain.User
		want string
	}{
		{
			name: "member",
			user: testUser,
			want: `{
				"user": {"id": "019a5b1e-7c2d-7000-8000-4b1d2c3e4f50", "username": "ana", "isServerAdmin": false},
				"permissions": [],
				"projects": [{"id": "019a5b1e-7c2d-7000-8000-4b1d2c3e4f51", "name": "default", "role": "editor",
					"permissions": ["runs:read", "tasks:read", "schedules:read", "workers:read", "runs:cancel", "schedules:write"]}]
			}`,
		},
		{
			name: "server admin",
			user: domain.User{ID: testUser.ID, Username: "ana", IsServerAdmin: true},
			want: `{
				"user": {"id": "019a5b1e-7c2d-7000-8000-4b1d2c3e4f50", "username": "ana", "isServerAdmin": true},
				"permissions": ["projects:create", "users:read", "users:write"],
				"projects": [{"id": "019a5b1e-7c2d-7000-8000-4b1d2c3e4f51", "name": "default", "role": "editor",
					"permissions": ["runs:read", "tasks:read", "schedules:read", "workers:read", "runs:cancel", "schedules:write"]}]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := &fakeAccounts{
				user:  tt.user,
				roles: []domain.ProjectRole{{ProjectID: projectID, ProjectName: "default", Role: domain.RoleEditor}},
			}
			h := newAPIServer(t, accounts)

			rec := do(t, h, call{method: http.MethodGet, path: "/api/v1/me", token: testToken})

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.JSONEq(t, tt.want, rec.Body.String())
		})
	}
}

func TestChangeMyPassword(t *testing.T) {
	accounts := &fakeAccounts{user: testUser}
	h := newAPIServer(t, accounts)

	rec := do(t, h, call{method: http.MethodPut, path: "/api/v1/me/password", token: testToken, body: `{"currentPassword":"admin","newPassword":"Secret-123"}`})

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, "Secret-123", accounts.newPassword)
}

func TestChangeMyPasswordRefused(t *testing.T) {
	accounts := &fakeAccounts{
		user:      testUser,
		changeErr: apierr.Invalid(apierr.CodeInvalidField, "current password is wrong").WithDetail("field", "currentPassword"),
	}
	h := newAPIServer(t, accounts)

	rec := do(t, h, call{method: http.MethodPut, path: "/api/v1/me/password", token: testToken, body: `{"currentPassword":"x","newPassword":"Secret-123"}`})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	code, field := decodeError(t, rec)
	assert.Equal(t, "invalid_field", code)
	assert.Equal(t, "currentPassword", field)
}

func TestRequestsRefusedBeforeTheHandler(t *testing.T) {
	tests := []struct {
		name      string
		call      call
		wantCode  int
		wantError string
	}{
		{name: "no session", call: call{method: http.MethodGet, path: "/api/v1/me"}, wantCode: http.StatusUnauthorized, wantError: "unauthenticated"},
		{name: "ended session", call: call{method: http.MethodGet, path: "/api/v1/me", token: "old"}, wantCode: http.StatusUnauthorized, wantError: "unauthenticated"},
		{name: "cross-origin write", call: call{method: http.MethodPost, path: "/api/v1/auth/logout", token: testToken, origin: "https://evil.example"}, wantCode: http.StatusForbidden, wantError: "permission_denied"},
		{name: "unknown path", call: call{method: http.MethodGet, path: "/api/v1/nothing", token: testToken}, wantCode: http.StatusNotFound, wantError: "not_found"},
		{name: "wrong method", call: call{method: http.MethodDelete, path: "/api/v1/me", token: testToken}, wantCode: http.StatusMethodNotAllowed, wantError: "invalid_request"},
		{name: "body too large", call: call{method: http.MethodPost, path: "/api/v1/auth/login", body: `{"login":"` + strings.Repeat("a", maxBodyBytes) + `","password":"x"}`}, wantCode: http.StatusRequestEntityTooLarge, wantError: "payload_too_large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newAPIServer(t, &fakeAccounts{user: testUser})

			rec := do(t, h, tt.call)

			assert.Equal(t, tt.wantCode, rec.Code)
			code, _ := decodeError(t, rec)
			assert.Equal(t, tt.wantError, code, rec.Body.String())
		})
	}
}
