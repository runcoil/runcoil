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
	ana := domain.User{ID: uuid.NewV7(), Username: "ana", Email: ptr("ana@example.com"), PasswordHash: hashPassword(t, "admin")}
	store := &fakeUserStore{users: []domain.User{ana}}
	s := newService(store)

	for _, login := range []string{"ana", "Ana", "ANA@example.com"} {
		result, err := s.Login(context.Background(), login, "admin")
		require.NoError(t, err, login)

		assert.Equal(t, ana, result.User)
		assert.True(t, result.PasswordChangeSuggested)
		assert.Equal(t, hashToken(result.Token), store.created)
		assert.Equal(t, ana.ID, store.createdFor)
		assert.Equal(t, ana.PasswordHash, store.createdWith)
		assert.WithinDuration(t, time.Now().Add(testConfig.SessionMaxLifetime), result.ExpiresAt, time.Minute)
	}
}

func TestLoginSuggestsNoChangeForAnotherPassword(t *testing.T) {
	store := &fakeUserStore{users: []domain.User{{ID: uuid.NewV7(), Username: "ana", PasswordHash: hashPassword(t, "Secret-1")}}}

	result, err := newService(store).Login(context.Background(), "ana", "Secret-1")

	require.NoError(t, err)
	assert.False(t, result.PasswordChangeSuggested)
}

func TestLoginRefused(t *testing.T) {
	store := &fakeUserStore{users: []domain.User{
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

func TestLoginRefusedWhenThePasswordChangedMeanwhile(t *testing.T) {
	store := &fakeUserStore{
		users:     []domain.User{{ID: uuid.NewV7(), Username: "ana", PasswordHash: hashPassword(t, "Secret-1")}},
		createErr: domain.ErrUserNotFound,
	}

	_, err := newService(store).Login(context.Background(), "ana", "Secret-1")

	assertUnauthenticated(t, err)
}
