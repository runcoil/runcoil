package account

import (
	"context"
	"sync"
	"time"
	"uuid"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

// The errors the service returns to clients.
var (
	ErrWrongLogin       = apierr.Unauthenticated(apierr.CodeUnauthenticated, "wrong login or password")
	ErrNotSignedIn      = apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in")
	ErrPermissionDenied = apierr.PermissionDenied(apierr.CodePermissionDenied, "permission denied")
)

// UserStore keeps the users.
type UserStore interface {
	GetUserByUsername(ctx context.Context, username string) (domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (domain.User, error)
	UpdatePasswordAndDeleteOtherSessions(ctx context.Context, userID uuid.UUID, passwordHash string, keepTokenHash []byte) error
}

// SessionStore keeps the sessions.
type SessionStore interface {
	CreateSession(ctx context.Context, tokenHash []byte, userID uuid.UUID, passwordHash string, maxLifetime time.Duration) error
	GetSessionUserAndTouch(ctx context.Context, tokenHash []byte, maxInactive time.Duration) (domain.User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
}

// ProjectStore keeps the projects and their members.
type ProjectStore interface {
	ListProjects(ctx context.Context) ([]domain.ProjectRole, error)
	ListProjectsWithRoleByUser(ctx context.Context, userID uuid.UUID) ([]domain.ProjectRole, error)
}

// Service signs users in and out, manages their passwords and decides what they may do.
type Service struct {
	cfg          Config
	userStore    UserStore
	sessionStore SessionStore
	projectStore ProjectStore
	now          func() time.Time

	bootstrapAdminPassword string
	dummyHash              func() string
}

// New creates the account service. A login with bootstrapAdminPassword is asked to
// change it.
func New(cfg Config, userStore UserStore, sessionStore SessionStore, projectStore ProjectStore, bootstrapAdminPassword string) *Service {
	return &Service{
		cfg:                    cfg,
		userStore:              userStore,
		sessionStore:           sessionStore,
		projectStore:           projectStore,
		now:                    time.Now,
		bootstrapAdminPassword: bootstrapAdminPassword,
		dummyHash: sync.OnceValue(func() string {
			hash, _ := HashPassword(context.Background(), "")
			return hash
		}),
	}
}
