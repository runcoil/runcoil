package account

import (
	"context"
	"errors"
	"time"
	"uuid"

	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/apperr"
	"github.com/runcoil/runcoil/internal/domain"
)

// The errors the service returns to clients.
var (
	ErrWrongLogin       = apperr.Unauthenticated(apperr.CodeUnauthenticated, "wrong login or password")
	ErrNotSignedIn      = apperr.Unauthenticated(apperr.CodeUnauthenticated, "not signed in")
	ErrPermissionDenied = apperr.PermissionDenied(apperr.CodePermissionDenied, "permission denied")
	ErrProjectNotFound  = apperr.NotFound(apperr.CodeNotFound, "project not found")
	ErrUserNotFound     = apperr.NotFound(apperr.CodeNotFound, "user not found")
	ErrMemberNotFound   = apperr.NotFound(apperr.CodeNotFound, "user is not a member of the project")
	ErrUsernameTaken    = apperr.Conflict(apperr.CodeAlreadyExists, "username is taken").WithDetail("field", "username")
	ErrEmailTaken       = apperr.Conflict(apperr.CodeAlreadyExists, "email is taken").WithDetail("field", "email")
	ErrProjectNameTaken = apperr.Conflict(apperr.CodeAlreadyExists, "project name is taken").WithDetail("field", "name")
	ErrMemberExists     = apperr.Conflict(apperr.CodeAlreadyExists, "user is already a member of the project").WithDetail("field", "username")
	ErrNoSuchUsername   = apperr.Invalid(apperr.CodeInvalidRequest, "no user has this username").WithDetail("field", "username")
	ErrLastServerAdmin  = apperr.Conflict(apperr.CodeLastServerAdmin, "the last active server admin cannot lose the role or be disabled")
)

// UserStore keeps the users.
type UserStore interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (domain.User, error)
	GetUserByUsername(ctx context.Context, username string) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	UpdatePasswordAndDeleteOtherSessions(ctx context.Context, userID uuid.UUID, currentHash, newHash string, keepTokenHash []byte) error
	CreateUser(ctx context.Context, user domain.User) (domain.User, error)
	ListUsers(ctx context.Context, afterID *uuid.UUID, limit int32) ([]domain.User, error)
	UpdateUser(ctx context.Context, id uuid.UUID, update domain.UserUpdate) (domain.User, error)
	SetPasswordAndDeleteSessions(ctx context.Context, id uuid.UUID, passwordHash string) error
}

// SessionStore keeps the sessions.
type SessionStore interface {
	CreateSession(ctx context.Context, tokenHash []byte, userID uuid.UUID, passwordHash string, maxLifetime time.Duration) error
	GetSessionUserAndTouch(ctx context.Context, tokenHash []byte, maxInactive time.Duration) (domain.User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
}

// ProjectStore keeps the projects and their members.
type ProjectStore interface {
	CreateProject(ctx context.Context, project domain.Project) (domain.Project, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	ListProjectsByMember(ctx context.Context, userID uuid.UUID) ([]domain.Project, error)
	ListProjectsWithRoleByUser(ctx context.Context, userID uuid.UUID) ([]domain.ProjectRole, error)
	GetProjectRoleOfUser(ctx context.Context, projectID, userID uuid.UUID) (*domain.Role, error)
	ListMembers(ctx context.Context, projectID uuid.UUID) ([]domain.Member, error)
	AddMember(ctx context.Context, projectID, userID uuid.UUID, role domain.Role) error
	UpdateMemberRole(ctx context.Context, projectID, userID uuid.UUID, role domain.Role) (domain.Member, error)
	RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error
}

const minSessionMaxInactive = 5 * time.Minute

// Config is the configuration of the accounts and their sessions.
type Config struct {
	SessionMaxLifetime time.Duration
	SessionMaxInactive time.Duration

	BootstrapAdminUsername string
	BootstrapAdminPassword string
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.SessionMaxLifetime, "session.max-lifetime", 30*24*time.Hour, "Longest time a dashboard session lasts.")
	fs.DurationVar(&c.SessionMaxInactive, "session.max-inactive", 7*24*time.Hour, "Time without activity after which a dashboard session ends.")
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.SessionMaxLifetime <= 0 {
		return errors.New("max lifetime must be positive")
	}

	// A session records its activity at most once a minute, so a shorter limit would end
	// sessions in use.
	if c.SessionMaxInactive < minSessionMaxInactive {
		return errors.New("max inactive must be at least 5m")
	}

	return nil
}

// Service signs users in and out, manages their passwords and decides what they may do.
type Service struct {
	cfg          Config
	userStore    UserStore
	sessionStore SessionStore
	projectStore ProjectStore
	now          func() time.Time

	dummyHash string
}

// New creates the account service.
func New(cfg Config, userStore UserStore, sessionStore SessionStore, projectStore ProjectStore) *Service {
	dummyHash, _ := HashPassword(context.Background(), "")

	return &Service{
		cfg:          cfg,
		userStore:    userStore,
		sessionStore: sessionStore,
		projectStore: projectStore,
		now:          time.Now,
		dummyHash:    dummyHash,
	}
}
