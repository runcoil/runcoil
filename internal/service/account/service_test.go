package account

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

type fakeUserStore struct {
	users    []domain.User
	sessions map[string]domain.Session
	roles    []domain.ProjectRole
	projects []domain.ProjectRole

	createErr error

	created       []byte
	createdFor    uuid.UUID
	createdWith   string
	touched       []byte
	deleted       []byte
	passwordHash  string
	keptTokenHash []byte
}

func (f *fakeUserStore) UserByUsername(_ context.Context, username string) (domain.User, error) {
	for _, u := range f.users {
		if u.Username == username {
			return u, nil
		}
	}

	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeUserStore) UserByEmail(_ context.Context, email string) (domain.User, error) {
	for _, u := range f.users {
		if u.Email != nil && *u.Email == email {
			return u, nil
		}
	}

	return domain.User{}, domain.ErrUserNotFound
}

func (f *fakeUserStore) CreateSession(_ context.Context, tokenHash []byte, userID uuid.UUID, passwordHash string, _ time.Duration) error {
	if f.createErr != nil {
		return f.createErr
	}

	f.created = tokenHash
	f.createdFor = userID
	f.createdWith = passwordHash

	return nil
}

func (f *fakeUserStore) SessionByToken(_ context.Context, tokenHash []byte, _ time.Duration) (domain.Session, error) {
	session, ok := f.sessions[string(tokenHash)]
	if !ok {
		return domain.Session{}, domain.ErrSessionNotFound
	}

	return session, nil
}

func (f *fakeUserStore) TouchSession(_ context.Context, tokenHash []byte) error {
	f.touched = tokenHash
	return nil
}

func (f *fakeUserStore) DeleteSession(_ context.Context, tokenHash []byte) error {
	f.deleted = tokenHash
	return nil
}

func (f *fakeUserStore) ChangePassword(_ context.Context, _ uuid.UUID, passwordHash string, keepTokenHash []byte) error {
	f.passwordHash = passwordHash
	f.keptTokenHash = keepTokenHash

	return nil
}

func (f *fakeUserStore) ProjectRoles(context.Context, uuid.UUID) ([]domain.ProjectRole, error) {
	return f.roles, nil
}

func (f *fakeUserStore) Projects(context.Context) ([]domain.ProjectRole, error) {
	return f.projects, nil
}

var testConfig = Config{
	SessionMaxLifetime: 30 * 24 * time.Hour,
	SessionMaxInactive: 7 * 24 * time.Hour,
}

func newService(store UserStore) *Service {
	return New(testConfig, store, "admin")
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()

	hash, err := HashPassword(context.Background(), password)
	require.NoError(t, err)

	return hash
}

func ptr[T any](v T) *T {
	return &v
}

func assertUnauthenticated(t *testing.T, err error) {
	t.Helper()

	apiErr, ok := errors.AsType[*apierr.Error](err)
	require.True(t, ok, err)
	assert.Equal(t, apierr.KindUnauthenticated, apiErr.Kind)
	assert.Equal(t, apierr.CodeUnauthenticated, apiErr.Code)
}
