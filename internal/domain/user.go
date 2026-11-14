package domain

import (
	"time"
	"uuid"
)

// User is a dashboard account.
type User struct {
	ID            uuid.UUID
	Username      string
	Email         *string
	PasswordHash  string
	IsServerAdmin bool
	DisabledAt    *time.Time
	CreatedAt     time.Time
}

// UpdateUserParams is a change to the user with ID. A nil field stays as it is.
type UpdateUserParams struct {
	ID            uuid.UUID
	Email         *string
	RemoveEmail   bool
	IsServerAdmin *bool
	Disabled      *bool
}

// Role is a user's role in a project.
type Role string

// The roles of a project member.
const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// Permission is what a user may do, as resource:verb.
type Permission string

// The permissions of the dashboard API.
const (
	PermissionRunsRead       Permission = "runs:read"
	PermissionRunsCancel     Permission = "runs:cancel"
	PermissionTasksRead      Permission = "tasks:read"
	PermissionSchedulesRead  Permission = "schedules:read"
	PermissionSchedulesWrite Permission = "schedules:write"
	PermissionWorkersRead    Permission = "workers:read"
	PermissionMembersRead    Permission = "members:read"
	PermissionMembersWrite   Permission = "members:write"
	PermissionAPIKeysRead    Permission = "api_keys:read"  //nolint:gosec // A permission name, not a credential.
	PermissionAPIKeysWrite   Permission = "api_keys:write" //nolint:gosec // A permission name, not a credential.
	PermissionProjectsCreate Permission = "projects:create"
	PermissionUsersRead      Permission = "users:read"
	PermissionUsersWrite     Permission = "users:write"
)

// Permissions lists every Permission.
var Permissions = []Permission{
	PermissionRunsRead,
	PermissionRunsCancel,
	PermissionTasksRead,
	PermissionSchedulesRead,
	PermissionSchedulesWrite,
	PermissionWorkersRead,
	PermissionMembersRead,
	PermissionMembersWrite,
	PermissionAPIKeysRead,
	PermissionAPIKeysWrite,
	PermissionProjectsCreate,
	PermissionUsersRead,
	PermissionUsersWrite,
}
