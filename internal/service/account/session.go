package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/runcoil/runcoil/internal/domain"
)

const sessionTokenLen = 32

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
		user, err = s.userStore.GetUserByEmail(ctx, login)
	} else {
		user, err = s.userStore.GetUserByUsername(ctx, login)
	}

	if errors.Is(err, domain.ErrUserNotFound) {
		_, _ = VerifyPassword(ctx, s.dummyHash(), password)
		return LoginResult{}, ErrWrongLogin
	}

	if err != nil {
		return LoginResult{}, fmt.Errorf("get user: %w", err)
	}

	ok, err := VerifyPassword(ctx, user.PasswordHash, password)
	if err != nil {
		return LoginResult{}, fmt.Errorf("verify password: %w", err)
	}

	if !ok || user.DisabledAt != nil {
		return LoginResult{}, ErrWrongLogin
	}

	token := newSessionToken()

	err = s.sessionStore.CreateSession(ctx, hashToken(token), user.ID, user.PasswordHash, s.cfg.SessionMaxLifetime)
	if errors.Is(err, domain.ErrUserNotFound) {
		return LoginResult{}, ErrWrongLogin
	}

	if err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}

	return LoginResult{
		Token:                   token,
		ExpiresAt:               s.now().Add(s.cfg.SessionMaxLifetime),
		User:                    user,
		PasswordChangeSuggested: password == s.bootstrapAdminPassword,
	}, nil
}

// AuthenticateSession returns the user of a session token and records the activity.
func (s *Service) AuthenticateSession(ctx context.Context, token string) (domain.User, error) {
	user, err := s.sessionStore.GetSessionUserAndTouch(ctx, hashToken(token), s.cfg.SessionMaxInactive)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return domain.User{}, ErrNotSignedIn
	}

	if err != nil {
		return domain.User{}, fmt.Errorf("get session user: %w", err)
	}

	return user, nil
}

// Logout ends the session of token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if err := s.sessionStore.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
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
