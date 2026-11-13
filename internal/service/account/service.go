package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"uuid"

	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

const (
	sessionTokenLen   = 32
	sessionTouchEvery = time.Minute
)

var (
	errWrongLogin = apierr.Unauthenticated(apierr.CodeUnauthenticated, "wrong login or password")
	errNoSession  = apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in")
)

// Config is the configuration of the accounts and their sessions.
type Config struct {
	SessionMaxLifetime time.Duration
	SessionMaxInactive time.Duration

	// BootstrapAdminPassword is the password of the first server admin. A login with it
	// is asked to change it.
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

	if c.SessionMaxInactive <= 0 {
		return errors.New("max inactive must be positive")
	}

	return nil
}

// UserStore keeps the users, their sessions and their project roles.
type UserStore interface {
	UserByUsername(ctx context.Context, username string) (domain.User, error)
	UserByEmail(ctx context.Context, email string) (domain.User, error)
	CreateSession(ctx context.Context, tokenHash []byte, userID uuid.UUID, maxLifetime, maxInactive time.Duration) error
	SessionByToken(ctx context.Context, tokenHash []byte, maxInactive time.Duration) (domain.Session, error)
	TouchSession(ctx context.Context, tokenHash []byte) error
	DeleteSession(ctx context.Context, tokenHash []byte) error
	ChangePassword(ctx context.Context, userID uuid.UUID, passwordHash string, keepTokenHash []byte) error
	ProjectRoles(ctx context.Context, userID uuid.UUID) ([]domain.ProjectRole, error)
	Projects(ctx context.Context) ([]domain.ProjectRole, error)
	MemberRole(ctx context.Context, projectID, userID uuid.UUID) (domain.Role, error)
}

// Service signs users in and out and manages their passwords.
type Service struct {
	cfg       Config
	userStore UserStore
	now       func() time.Time

	dummyHash func() string
}

// New creates the account service.
func New(cfg Config, userStore UserStore) *Service {
	return &Service{
		cfg:       cfg,
		userStore: userStore,
		now:       time.Now,
		dummyHash: sync.OnceValue(func() string { return HashPassword("") }),
	}
}

// LoginResult is a new session and its user.
type LoginResult struct {
	Token                   string
	ExpiresAt               time.Time
	User                    domain.User
	PasswordChangeSuggested bool
}

// Login checks a username or an email and its password, and starts a session.
func (s *Service) Login(ctx context.Context, login, password string) (LoginResult, error) {
	login = strings.ToLower(login)

	var user domain.User
	var err error
	if strings.Contains(login, "@") {
		user, err = s.userStore.UserByEmail(ctx, login)
	} else {
		user, err = s.userStore.UserByUsername(ctx, login)
	}

	if errors.Is(err, domain.ErrUserNotFound) {
		_, _ = VerifyPassword(s.dummyHash(), password)
		return LoginResult{}, errWrongLogin
	}

	if err != nil {
		return LoginResult{}, fmt.Errorf("get user: %w", err)
	}

	ok, err := VerifyPassword(user.PasswordHash, password)
	if err != nil {
		return LoginResult{}, fmt.Errorf("verify password: %w", err)
	}

	if !ok || user.DisabledAt != nil {
		return LoginResult{}, errWrongLogin
	}

	token := newSessionToken()
	if err := s.userStore.CreateSession(ctx, hashToken(token), user.ID, s.cfg.SessionMaxLifetime, s.cfg.SessionMaxInactive); err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}

	return LoginResult{
		Token:                   token,
		ExpiresAt:               s.now().Add(s.cfg.SessionMaxLifetime),
		User:                    user,
		PasswordChangeSuggested: password == s.cfg.BootstrapAdminPassword,
	}, nil
}

// Authenticate returns the user of a session token.
func (s *Service) Authenticate(ctx context.Context, token string) (domain.User, error) {
	tokenHash := hashToken(token)

	session, err := s.userStore.SessionByToken(ctx, tokenHash, s.cfg.SessionMaxInactive)
	if errors.Is(err, domain.ErrNoSession) {
		return domain.User{}, errNoSession
	}

	if err != nil {
		return domain.User{}, fmt.Errorf("get session: %w", err)
	}

	if s.now().Sub(session.LastSeenAt) >= sessionTouchEvery {
		if err := s.userStore.TouchSession(ctx, tokenHash); err != nil {
			return domain.User{}, fmt.Errorf("touch session: %w", err)
		}
	}

	return session.User, nil
}

// Logout ends the session of token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if err := s.userStore.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// ChangePassword sets the password of user, who is signed in with token, and ends the
// user's other sessions.
func (s *Service) ChangePassword(ctx context.Context, user domain.User, token, currentPassword, newPassword string) error {
	ok, err := VerifyPassword(user.PasswordHash, currentPassword)
	if err != nil {
		return fmt.Errorf("verify password: %w", err)
	}

	if !ok {
		return apierr.Invalid(apierr.CodeInvalidField, "current password is wrong").WithDetail("field", "currentPassword")
	}

	if err := ValidatePassword(newPassword); err != nil {
		return apierr.Invalid(apierr.CodeInvalidField, "new password "+err.Error()).WithDetail("field", "newPassword")
	}

	if err := s.userStore.ChangePassword(ctx, user.ID, HashPassword(newPassword), hashToken(token)); err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	return nil
}

// ProjectRoles returns the projects user can open, with the role in each. A server admin
// is admin in every project.
func (s *Service) ProjectRoles(ctx context.Context, user domain.User) ([]domain.ProjectRole, error) {
	if user.IsServerAdmin {
		return s.userStore.Projects(ctx)
	}

	return s.userStore.ProjectRoles(ctx, user.ID)
}

// Role returns the role of user in projectID. It reports false when user is not a member.
func (s *Service) Role(ctx context.Context, user domain.User, projectID uuid.UUID) (domain.Role, bool, error) {
	if user.IsServerAdmin {
		return domain.RoleAdmin, true, nil
	}

	role, err := s.userStore.MemberRole(ctx, projectID, user.ID)
	if errors.Is(err, domain.ErrMemberNotFound) {
		return "", false, nil
	}

	if err != nil {
		return "", false, fmt.Errorf("get member role: %w", err)
	}

	return role, true, nil
}

func newSessionToken() string {
	b := make([]byte, sessionTokenLen)
	_, _ = rand.Read(b)

	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
