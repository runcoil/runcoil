package account

import (
	"context"
	"sync"
	"time"
	"uuid"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

var (
	errWrongLogin       = apierr.Unauthenticated(apierr.CodeUnauthenticated, "wrong login or password")
	errNoSession        = apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in")
	errPermissionDenied = apierr.PermissionDenied(apierr.CodePermissionDenied, "permission denied")
)

// UserStore keeps the users, their sessions and their project roles.
type UserStore interface {
	UserByUsername(ctx context.Context, username string) (domain.User, error)
	UserByEmail(ctx context.Context, email string) (domain.User, error)
	CreateSession(ctx context.Context, tokenHash []byte, userID uuid.UUID, passwordHash string, maxLifetime time.Duration) error
	SessionByToken(ctx context.Context, tokenHash []byte, maxInactive time.Duration) (domain.Session, error)
	TouchSession(ctx context.Context, tokenHash []byte) error
	DeleteSession(ctx context.Context, tokenHash []byte) error
	ChangePassword(ctx context.Context, userID uuid.UUID, passwordHash string, keepTokenHash []byte) error
	ProjectRoles(ctx context.Context, userID uuid.UUID) ([]domain.ProjectRole, error)
	Projects(ctx context.Context) ([]domain.ProjectRole, error)
}

// Service signs users in and out, manages their passwords and decides what they may do.
type Service struct {
	cfg       Config
	userStore UserStore
	now       func() time.Time

	bootstrapAdminPassword string
	dummyHash              func() string
}

// New creates the account service. A login with bootstrapAdminPassword is asked to
// change it.
func New(cfg Config, userStore UserStore, bootstrapAdminPassword string) *Service {
	return &Service{
		cfg:                    cfg,
		userStore:              userStore,
		now:                    time.Now,
		bootstrapAdminPassword: bootstrapAdminPassword,
		dummyHash: sync.OnceValue(func() string {
			hash, _ := HashPassword(context.Background(), "")
			return hash
		}),
	}
}
