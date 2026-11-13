package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/openapi"
)

func loadSpec(t *testing.T, data []byte) *openapi3.T {
	t.Helper()

	spec, err := openapi3.NewLoader().LoadFromData(data)
	require.NoError(t, err)

	return spec
}

func TestEveryOperationHasAPermission(t *testing.T) {
	permissions, err := Permissions(loadSpec(t, openapi.Spec))
	require.NoError(t, err)

	operations := reflect.TypeFor[gen.StrictServerInterface]()
	for i := range operations.NumMethod() {
		assert.Contains(t, permissions, operations.Method(i).Name)
	}

	assert.Equal(t, permissionPublic, permissions["Login"])
	assert.Equal(t, permissionSelf, permissions["GetMe"])
}

func TestDomainPermissionsMatchTheSpec(t *testing.T) {
	spec := loadSpec(t, openapi.Spec)

	var want []domain.Permission
	for _, value := range spec.Components.Schemas["Permission"].Value.Enum {
		want = append(want, domain.Permission(value.(string)))
	}

	assert.Equal(t, want, domain.Permissions)
}

func TestPermissionsRefusesABadSpec(t *testing.T) {
	tests := []struct {
		name       string
		permission string
		want       string
	}{
		{name: "missing", want: "GET /x has no x-runcoil-permission"},
		{name: "unknown", permission: "\n      x-runcoil-permission: runs:delete", want: `GET /x has an unknown permission "runs:delete"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := loadSpec(t, []byte(`
openapi: 3.1.0
info:
  title: t
  version: "1"
paths:
  /x:
    get:
      operationId: getX`+tt.permission+`
      responses:
        "204":
          description: ok
`))

			_, err := Permissions(spec)

			assert.EqualError(t, err, tt.want)
		})
	}
}

type fakeAuthorizer struct {
	asked domain.Permission
}

func (f *fakeAuthorizer) Authorize(user domain.User, permission domain.Permission) error {
	f.asked = permission
	if user.IsServerAdmin {
		return nil
	}

	return apierr.PermissionDenied(apierr.CodePermissionDenied, "permission denied")
}

func TestCheckPermission(t *testing.T) {
	permissions := map[string]string{
		"Login":       permissionPublic,
		"GetMe":       permissionSelf,
		"CreateUser":  string(domain.PermissionUsersWrite),
		"ListMembers": string(domain.PermissionMembersRead),
	}
	signedIn := context.WithValue(context.Background(), callerKey{}, Caller{User: domain.User{Username: "ana"}})
	admin := context.WithValue(context.Background(), callerKey{}, Caller{User: domain.User{IsServerAdmin: true}})

	tests := []struct {
		name      string
		operation string
		ctx       context.Context
		allowed   bool
		wantKind  apierr.Kind
		wantAsked domain.Permission
	}{
		{name: "public without a caller", operation: "Login", ctx: context.Background(), allowed: true},
		{name: "self without a caller", operation: "GetMe", ctx: context.Background(), wantKind: apierr.KindUnauthenticated},
		{name: "self signed in", operation: "GetMe", ctx: signedIn, allowed: true},
		{name: "permission without a caller", operation: "CreateUser", ctx: context.Background(), wantKind: apierr.KindUnauthenticated},
		{name: "permission refused", operation: "ListMembers", ctx: signedIn, wantKind: apierr.KindPermissionDenied, wantAsked: domain.PermissionMembersRead},
		{name: "permission granted", operation: "CreateUser", ctx: admin, allowed: true, wantAsked: domain.PermissionUsersWrite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := &fakeAuthorizer{}
			called := false
			next := func(context.Context, http.ResponseWriter, *http.Request, any) (any, error) {
				called = true
				return nil, nil
			}

			_, err := CheckPermission(permissions, authorizer)(next, tt.operation)(tt.ctx, nil, httptest.NewRequest(http.MethodGet, "/", nil), nil)

			assert.Equal(t, tt.wantAsked, authorizer.asked)

			if tt.allowed {
				require.NoError(t, err)
				assert.True(t, called)

				return
			}

			apiErr, ok := errors.AsType[*apierr.Error](err)
			require.True(t, ok, err)
			assert.Equal(t, tt.wantKind, apiErr.Kind)
			assert.False(t, called)
		})
	}
}

func TestCheckPermissionRefusesAnUnknownOperation(t *testing.T) {
	called := false
	next := func(context.Context, http.ResponseWriter, *http.Request, any) (any, error) {
		called = true
		return nil, nil
	}

	_, err := CheckPermission(map[string]string{}, &fakeAuthorizer{})(next, "Unknown")(context.Background(), nil, httptest.NewRequest(http.MethodGet, "/", nil), nil)

	assert.EqualError(t, err, "operation Unknown has no permission in the spec")
	assert.False(t, called)
}
