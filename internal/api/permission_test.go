package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"uuid"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/openapi"
)

func TestEveryOperationHasAPermission(t *testing.T) {
	spec, err := openapi3.NewLoader().LoadFromData(openapi.Spec)
	require.NoError(t, err)

	permissions, err := operationPermissions(spec)
	require.NoError(t, err)

	operations := reflect.TypeFor[gen.StrictServerInterface]()
	for i := range operations.NumMethod() {
		name := operations.Method(i).Name
		assert.Contains(t, permissions, name)
	}

	assert.Equal(t, permissionPublic, permissions["Login"])
}

func TestCheckPermissionRefusesAnUnknownOperation(t *testing.T) {
	s := &Service{permissions: map[string]string{}}
	called := false
	next := func(context.Context, http.ResponseWriter, *http.Request, any) (any, error) {
		called = true
		return nil, nil
	}

	_, err := s.checkPermission(next, "Unknown")(context.Background(), nil, httptest.NewRequest(http.MethodGet, "/", nil), nil)

	assert.Error(t, err)
	assert.False(t, called)
}

func TestOperationPermissionsRefusesAMissingOne(t *testing.T) {
	spec, err := openapi3.NewLoader().LoadFromData([]byte(`
openapi: 3.1.0
info: {title: t, version: "1"}
paths:
  /x:
    get:
      operationId: getX
      responses:
        "204": {description: ok}
`))
	require.NoError(t, err)

	_, err = operationPermissions(spec)

	assert.EqualError(t, err, "GET /x has no x-runcoil-permission")
}

func TestAuthorize(t *testing.T) {
	projectID := uuid.MustParse("019a5b1e-7c2d-7000-8000-4b1d2c3e4f51").String()

	tests := []struct {
		name       string
		user       domain.User
		role       domain.Role
		permission gen.Permission
		projectID  string
		allowed    bool
	}{
		{name: "viewer reads runs", user: testUser, role: domain.RoleViewer, permission: gen.RunsRead, projectID: projectID, allowed: true},
		{name: "viewer cannot cancel", user: testUser, role: domain.RoleViewer, permission: gen.RunsCancel, projectID: projectID},
		{name: "editor cancels", user: testUser, role: domain.RoleEditor, permission: gen.RunsCancel, projectID: projectID, allowed: true},
		{name: "editor cannot add members", user: testUser, role: domain.RoleEditor, permission: gen.MembersWrite, projectID: projectID},
		{name: "admin makes keys", user: testUser, role: domain.RoleAdmin, permission: gen.ApiKeysWrite, projectID: projectID, allowed: true},
		{name: "admin cannot create projects", user: testUser, role: domain.RoleAdmin, permission: gen.ProjectsCreate, projectID: projectID},
		{name: "not a member", user: testUser, permission: gen.RunsRead, projectID: projectID},
		{name: "no project", user: testUser, role: domain.RoleAdmin, permission: gen.UsersRead},
		{name: "server admin", user: domain.User{IsServerAdmin: true}, permission: gen.UsersWrite, allowed: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{accounts: &fakeAccounts{role: tt.role}}

			err := s.authorize(context.Background(), tt.user, tt.permission, tt.projectID)
			if tt.allowed {
				assert.NoError(t, err)
				return
			}

			assert.ErrorIs(t, err, errPermissionDenied)
		})
	}
}
