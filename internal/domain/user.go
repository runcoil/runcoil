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

// UserUpdate is a change to a user. A nil field stays as it is.
type UserUpdate struct {
	// Email is the new email; an empty string removes it.
	Email         *string
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

// ProjectRole is a project and the role a user has in it.
type ProjectRole struct {
	ProjectID   uuid.UUID
	ProjectName string
	Role        Role
}

// Project is a set of Tasks, Runs and API keys, and the users who may see them.
type Project struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// Member is a user with a role in a project.
type Member struct {
	UserID   uuid.UUID
	Username string
	Email    *string
	Role     Role
}
