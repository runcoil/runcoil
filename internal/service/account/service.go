package account

import (
	"context"
	"errors"
	"sync"
	"time"
	"uuid"

	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

// The errors the service returns to clients.
var (
	ErrWrongLogin  = apierr.Unauthenticated(apierr.CodeUnauthenticated, "wrong login or password")
	ErrNotSignedIn = apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in")
)

// UserStore keeps the users.
type UserStore interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (domain.User, error)
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

const minSessionMaxInactive = 5 * time.Minute

// Config is the configuration of the accounts and their sessions.
type Config struct {
	SessionMaxLifetime time.Duration
	SessionMaxInactive time.Duration

	// BootstrapAdminUsername and BootstrapAdminPassword are the seed's bootstrap admin,
	// which main copies here: a login as that user with that password is asked to change
	// it. They have no key of their own.
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

	dummyHash func() string
}

// New creates the account service.
func New(cfg Config, userStore UserStore, sessionStore SessionStore, projectStore ProjectStore) *Service {
	return &Service{
		cfg:          cfg,
		userStore:    userStore,
		sessionStore: sessionStore,
		projectStore: projectStore,
		now:          time.Now,
		dummyHash: sync.OnceValue(func() string {
			hash, _ := HashPassword(context.Background(), "")
			return hash
		}),
	}
}
