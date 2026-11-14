package api

import (
	"net/http"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apperr"
	"github.com/runcoil/runcoil/internal/domain"
)

var testAdmin = domain.User{ID: testUser.ID, Username: "admin", IsServerAdmin: true}

func TestListProjects(t *testing.T) {
	accountService := &fakeAccountService{user: testUser, projects: []domain.Project{{ID: testProjectID, Name: "default", CreatedAt: testTime}}}
	h := newAPIServer(t, accountService)

	rec := do(t, h, call{method: http.MethodGet, path: "/api/v1/projects", token: testToken})

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"projects":[{"id":"019a5b1e-7c2d-7000-8000-4b1d2c3e4f51","name":"default","createdAt":"2026-11-14T08:00:00Z"}]}`, rec.Body.String())

	rec = do(t, h, call{method: http.MethodGet, path: "/api/v1/projects"})

	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}

func TestCreateProject(t *testing.T) {
	h := newAPIServer(t, &fakeAccountService{user: testAdmin})

	rec := do(t, h, call{method: http.MethodPost, path: "/api/v1/projects", token: testToken, body: `{"name":"billing"}`})

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"id":"019a5b1e-7c2d-7000-8000-4b1d2c3e4f51","name":"billing","createdAt":"2026-11-14T08:00:00Z"}`, rec.Body.String())
}

func TestCreateProjectErrors(t *testing.T) {
	tests := []struct {
		name       string
		user       domain.User
		body       string
		manageErr  error
		wantCode   int
		wantError  string
		wantFields []string
	}{
		{name: "not a server admin", user: testUser, body: `{"name":"billing"}`, wantCode: http.StatusForbidden, wantError: "permission_denied"},
		{name: "empty name", user: testAdmin, body: `{"name":""}`, wantCode: http.StatusBadRequest, wantError: "invalid_request", wantFields: []string{"name"}},
		{name: "name taken", user: testAdmin, body: `{"name":"billing"}`, manageErr: apperr.Conflict(apperr.CodeAlreadyExists, "project name is taken").WithDetail("field", "name"), wantCode: http.StatusConflict, wantError: "already_exists", wantFields: []string{"name"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newAPIServer(t, &fakeAccountService{user: tt.user, manageErr: tt.manageErr})

			rec := do(t, h, call{method: http.MethodPost, path: "/api/v1/projects", token: testToken, body: tt.body})

			require.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			code, fields := decodeError(t, rec)
			assert.Equal(t, tt.wantError, code)
			assert.Equal(t, tt.wantFields, fields)
		})
	}
}

func TestProjectOperationsAskTheAuthorizer(t *testing.T) {
	accountService := &fakeAccountService{user: testUser, authorizeErr: apperr.NotFound(apperr.CodeNotFound, "project not found")}
	h := newAPIServer(t, accountService)

	rec := do(t, h, call{method: http.MethodGet, path: "/api/v1/projects/" + testProjectID.String() + "/members", token: testToken})

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	code, _ := decodeError(t, rec)
	assert.Equal(t, "not_found", code)
	assert.Equal(t, testProjectID, accountService.authorizedProject)
	assert.Equal(t, domain.PermissionMembersRead, accountService.authorizedPermission)

	rec = do(t, h, call{method: http.MethodGet, path: "/api/v1/projects/" + uuid.NewV7().String() + "x/members", token: testToken})

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}
