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
	memberOf map[uuid.UUID]domain.Role

	created       []byte
	createdFor    uuid.UUID
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

func (f *fakeUserStore) CreateSession(_ context.Context, tokenHash []byte, userID uuid.UUID, _ time.Duration) error {
	f.created = tokenHash
	f.createdFor = userID

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

func (f *fakeUserStore) MemberRole(_ context.Context, projectID, _ uuid.UUID) (domain.Role, error) {
	role, ok := f.memberOf[projectID]
	if !ok {
		return "", domain.ErrMemberNotFound
	}

	return role, nil
}

var testConfig = Config{
	SessionMaxLifetime: 30 * 24 * time.Hour,
	SessionMaxInactive: 7 * 24 * time.Hour,
}

func newService(store UserStore) *Service {
	return New(testConfig, store, "admin")
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

func TestLogin(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana", Email: ptr("ana@example.com"), PasswordHash: HashPassword("admin")}
	store := &fakeUserStore{users: []domain.User{ana}}
	s := newService(store)

	for _, login := range []string{"ana", "Ana", "ANA@example.com"} {
		result, err := s.Login(context.Background(), login, "admin")
		require.NoError(t, err, login)

		assert.Equal(t, ana, result.User)
		assert.True(t, result.PasswordChangeSuggested)
		assert.Equal(t, hashToken(result.Token), store.created)
		assert.Equal(t, ana.ID, store.createdFor)
		assert.WithinDuration(t, time.Now().Add(testConfig.SessionMaxLifetime), result.ExpiresAt, time.Minute)
	}
}

func TestLoginSuggestsNoChangeForAnotherPassword(t *testing.T) {
	store := &fakeUserStore{users: []domain.User{{ID: uuid.NewV7(), Username: "ana", PasswordHash: HashPassword("Secret-1")}}}

	result, err := newService(store).Login(context.Background(), "ana", "Secret-1")

	require.NoError(t, err)
	assert.False(t, result.PasswordChangeSuggested)
}

func TestLoginRefused(t *testing.T) {
	store := &fakeUserStore{users: []domain.User{
		{ID: uuid.NewV7(), Username: "ana", PasswordHash: HashPassword("Secret-1")},
		{ID: uuid.NewV7(), Username: "bob", PasswordHash: HashPassword("Secret-1"), DisabledAt: ptr(time.Now())},
	}}
	s := newService(store)

	for _, tt := range []struct{ login, password string }{
		{"ana", "secret-1"},
		{"nobody", "Secret-1"},
		{"bob", "Secret-1"},
	} {
		_, err := s.Login(context.Background(), tt.login, tt.password)
		assertUnauthenticated(t, err)
	}

	assert.Nil(t, store.created)
}

func TestAuthenticate(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana"}

	tests := []struct {
		name      string
		wantTouch bool
	}{
		{name: "seen just now"},
		{name: "seen a while ago", wantTouch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeUserStore{sessions: map[string]domain.Session{
				string(hashToken("token")): {User: ana, TouchDue: tt.wantTouch},
			}}

			user, err := newService(store).Authenticate(context.Background(), "token")

			require.NoError(t, err)
			assert.Equal(t, ana, user)

			if tt.wantTouch {
				assert.Equal(t, hashToken("token"), store.touched)
			} else {
				assert.Nil(t, store.touched)
			}
		})
	}
}

func TestAuthenticateWithoutSession(t *testing.T) {
	_, err := newService(&fakeUserStore{}).Authenticate(context.Background(), "token")

	assertUnauthenticated(t, err)
}

func TestLogout(t *testing.T) {
	store := &fakeUserStore{}

	require.NoError(t, newService(store).Logout(context.Background(), "token"))

	assert.Equal(t, hashToken("token"), store.deleted)
}

func TestChangePassword(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana", PasswordHash: HashPassword("admin")}
	store := &fakeUserStore{}

	err := newService(store).ChangePassword(context.Background(), ana, "token", "admin", "Secret-1")

	require.NoError(t, err)
	assert.Equal(t, hashToken("token"), store.keptTokenHash)

	ok, err := VerifyPassword(store.passwordHash, "Secret-1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestChangePasswordRefused(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana", PasswordHash: HashPassword("admin")}

	tests := []struct {
		name      string
		current   string
		new       string
		wantField string
	}{
		{name: "wrong current password", current: "Admin", new: "Secret-1", wantField: "currentPassword"},
		{name: "weak new password", current: "admin", new: "secret", wantField: "newPassword"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeUserStore{}

			err := newService(store).ChangePassword(context.Background(), ana, "token", tt.current, tt.new)

			apiErr, ok := errors.AsType[*apierr.Error](err)
			require.True(t, ok, err)
			assert.Equal(t, apierr.KindInvalid, apiErr.Kind)
			assert.Equal(t, apierr.CodeInvalidField, apiErr.Code)
			assert.Equal(t, tt.wantField, apiErr.Details["field"])
			assert.Empty(t, store.passwordHash)
		})
	}
}

func TestProjectRoles(t *testing.T) {
	member := []domain.ProjectRole{{ProjectID: uuid.NewV7(), ProjectName: "a", Role: domain.RoleViewer}}
	every := []domain.ProjectRole{{ProjectID: uuid.NewV7(), ProjectName: "b", Role: domain.RoleAdmin}}
	s := newService(&fakeUserStore{roles: member, projects: every})

	roles, err := s.ProjectRoles(context.Background(), domain.User{})
	require.NoError(t, err)
	assert.Equal(t, member, roles)

	roles, err = s.ProjectRoles(context.Background(), domain.User{IsServerAdmin: true})
	require.NoError(t, err)
	assert.Equal(t, every, roles)
}

func TestRole(t *testing.T) {
	projectID := uuid.NewV7()
	s := newService(&fakeUserStore{memberOf: map[uuid.UUID]domain.Role{projectID: domain.RoleEditor}})

	role, ok, err := s.Role(context.Background(), domain.User{}, projectID)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, domain.RoleEditor, role)

	_, ok, err = s.Role(context.Background(), domain.User{}, uuid.NewV7())
	require.NoError(t, err)
	assert.False(t, ok)

	role, ok, err = s.Role(context.Background(), domain.User{IsServerAdmin: true}, uuid.NewV7())
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, domain.RoleAdmin, role)
}

func TestConfigValidate(t *testing.T) {
	assert.NoError(t, testConfig.Validate())
	assert.EqualError(t, (&Config{SessionMaxInactive: time.Hour}).Validate(), "max lifetime must be positive")
	assert.EqualError(t, (&Config{SessionMaxLifetime: time.Hour}).Validate(), "max inactive must be positive")
}
