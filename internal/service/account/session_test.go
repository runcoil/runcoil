package account

import (
	"context"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/domain"
)

func TestLogin(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana", Email: ptr("ana@example.com"), PasswordHash: hashPassword(t, "Secret-1")}
	store := &fakeStore{users: []domain.User{ana}}
	s := newService(store)

	for _, login := range []string{"ana", "Ana", "ANA@example.com"} {
		result, err := s.Login(context.Background(), login, "Secret-1")
		require.NoError(t, err, login)

		assert.Equal(t, ana, result.User)
		assert.False(t, result.PasswordChangeSuggested)
		assert.Equal(t, hashToken(result.Token), store.created)
		assert.Equal(t, ana.ID, store.createdFor)
		assert.Equal(t, ana.PasswordHash, store.createdWith)
		assert.WithinDuration(t, time.Now().Add(testConfig.SessionMaxLifetime), result.ExpiresAt, time.Minute)
	}
}

func TestLoginSuggestsAChangeOfTheBootstrapPassword(t *testing.T) {
	store := &fakeStore{users: []domain.User{
		{ID: uuid.NewV7(), Username: "admin", PasswordHash: hashPassword(t, "admin")},
		{ID: uuid.NewV7(), Username: "ana", PasswordHash: hashPassword(t, "admin")},
	}}
	s := newService(store)

	result, err := s.Login(context.Background(), "admin", "admin")
	require.NoError(t, err)
	assert.True(t, result.PasswordChangeSuggested)

	result, err = s.Login(context.Background(), "ana", "admin")
	require.NoError(t, err)
	assert.False(t, result.PasswordChangeSuggested)
}

func TestLoginRefused(t *testing.T) {
	store := &fakeStore{users: []domain.User{
		{ID: uuid.NewV7(), Username: "ana", PasswordHash: hashPassword(t, "Secret-1")},
		{ID: uuid.NewV7(), Username: "bob", PasswordHash: hashPassword(t, "Secret-1"), DisabledAt: ptr(time.Now())},
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

func TestAuthenticateSession(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana"}
	store := &fakeStore{sessions: map[string]domain.User{string(hashToken("token")): ana}}

	user, err := newService(store).AuthenticateSession(context.Background(), "token")

	require.NoError(t, err)
	assert.Equal(t, ana, user)
}

func TestAuthenticateSessionWithoutSession(t *testing.T) {
	_, err := newService(&fakeStore{}).AuthenticateSession(context.Background(), "token")

	assertUnauthenticated(t, err)
}

func TestLogout(t *testing.T) {
	store := &fakeStore{}

	require.NoError(t, newService(store).Logout(context.Background(), "token"))

	assert.Equal(t, hashToken("token"), store.deleted)
}

func TestLoginRefusedWhenThePasswordChangedMeanwhile(t *testing.T) {
	store := &fakeStore{
		users:     []domain.User{{ID: uuid.NewV7(), Username: "ana", PasswordHash: hashPassword(t, "Secret-1")}},
		createErr: domain.ErrUserNotFound,
	}

	_, err := newService(store).Login(context.Background(), "ana", "Secret-1")

	assertUnauthenticated(t, err)
}
