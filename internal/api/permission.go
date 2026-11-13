package api

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/domain"
)

const (
	permissionExtension = "x-runcoil-permission"
	permissionSelf      = "self"
)

var viewerPermissions = []gen.Permission{gen.RunsRead, gen.TasksRead, gen.SchedulesRead, gen.WorkersRead}

var editorPermissions = append(slices.Clone(viewerPermissions), gen.RunsCancel, gen.SchedulesWrite)

var adminPermissions = append(slices.Clone(editorPermissions), gen.MembersRead, gen.MembersWrite, gen.ApiKeysRead, gen.ApiKeysWrite)

var rolePermissions = map[domain.Role][]gen.Permission{
	domain.RoleViewer: viewerPermissions,
	domain.RoleEditor: editorPermissions,
	domain.RoleAdmin:  adminPermissions,
}

var serverAdminPermissions = []gen.Permission{gen.ProjectsCreate, gen.UsersRead, gen.UsersWrite}

func roleHas(role domain.Role, permission gen.Permission) bool {
	return slices.Contains(rolePermissions[role], permission)
}

func operationPermissions(spec *openapi3.T) (map[string]string, error) {
	permissions := make(map[string]string)

	for path, item := range spec.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Security != nil && len(*op.Security) == 0 {
				continue
			}

			permission, _ := op.Extensions[permissionExtension].(string)
			if permission == "" {
				return nil, fmt.Errorf("%s %s has no %s", method, path, permissionExtension)
			}

			if permission != permissionSelf && !gen.Permission(permission).Valid() {
				return nil, fmt.Errorf("%s %s has an unknown permission %q", method, path, permission)
			}

			permissions[goName(op.OperationID)] = permission
		}
	}

	if len(permissions) == 0 {
		return nil, errors.New("no operation has a permission")
	}

	return permissions, nil
}

func goName(operationID string) string {
	if operationID == "" {
		return ""
	}

	return strings.ToUpper(operationID[:1]) + operationID[1:]
}
