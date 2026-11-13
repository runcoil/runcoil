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
}

// Role is a user's role in a project.
type Role string

// The roles of a project member.
const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// ProjectRole is a project and the role a user has in it.
type ProjectRole struct {
	ProjectID   uuid.UUID
	ProjectName string
	Role        Role
}

// Session is a signed-in user's session. TouchDue is true when its last activity was
// recorded more than a minute ago.
type Session struct {
	User     User
	TouchDue bool
}
