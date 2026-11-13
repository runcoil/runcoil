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
		user, err = s.userStore.UserByEmail(ctx, login)
	} else {
		user, err = s.userStore.UserByUsername(ctx, login)
	}

	if errors.Is(err, domain.ErrUserNotFound) {
		_, _ = VerifyPassword(ctx, s.dummyHash(), password)
		return LoginResult{}, errWrongLogin
	}

	if err != nil {
		return LoginResult{}, fmt.Errorf("get user: %w", err)
	}

	ok, err := VerifyPassword(ctx, user.PasswordHash, password)
	if err != nil {
		return LoginResult{}, fmt.Errorf("verify password: %w", err)
	}

	if !ok || user.DisabledAt != nil {
		return LoginResult{}, errWrongLogin
	}

	token := newSessionToken()

	err = s.userStore.CreateSession(ctx, hashToken(token), user.ID, user.PasswordHash, s.cfg.SessionMaxLifetime)
	if errors.Is(err, domain.ErrUserNotFound) {
		return LoginResult{}, errWrongLogin
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

// Authenticate returns the user of a session token.
func (s *Service) Authenticate(ctx context.Context, token string) (domain.User, error) {
	tokenHash := hashToken(token)

	session, err := s.userStore.SessionByToken(ctx, tokenHash, s.cfg.SessionMaxInactive)
	if errors.Is(err, domain.ErrSessionNotFound) {
		return domain.User{}, errNoSession
	}

	if err != nil {
		return domain.User{}, fmt.Errorf("get session: %w", err)
	}

	if session.TouchDue {
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

func newSessionToken() string {
	b := make([]byte, sessionTokenLen)
	_, _ = rand.Read(b)

	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
