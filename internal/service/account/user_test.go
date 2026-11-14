package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apperr"
	"github.com/runcoil/runcoil/internal/domain"
)

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{name: "every allowed character", username: "ops.admin_1-a"},
		{name: "longest", username: strings.Repeat("a", 64)},
		{name: "empty", username: "", wantErr: true},
		{name: "too long", username: strings.Repeat("a", 65), wantErr: true},
		{name: "upper case", username: "Admin", wantErr: true},
		{name: "email", username: "admin@example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, "must be 1 to 64 characters of a-z, 0-9, '.', '_' and '-'")
		})
	}
}

func TestGetUser(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana"}
	s := newService(&fakeStore{users: []domain.User{ana}})

	user, err := s.GetUser(context.Background(), ana.ID)
	require.NoError(t, err)
	assert.Equal(t, ana, user)

	_, err = s.GetUser(context.Background(), uuid.NewV7())
	assert.ErrorIs(t, err, domain.ErrUserNotFound)
}

func TestValidateEmail(t *testing.T) {
	assert.NoError(t, ValidateEmail("ana@example.com"))
	assert.NoError(t, ValidateEmail(strings.Repeat("a", 242)+"@example.com"))

	for _, email := range []string{"", "ana", "Ana <ana@example.com>", "<ana@example.com>", " ana@example.com", strings.Repeat("a", 243) + "@example.com"} {
		assert.EqualError(t, ValidateEmail(email), "must be a plain address such as ana@example.com", email)
	}
}

func TestCreateUser(t *testing.T) {
	store := &fakeStore{}

	user, err := newService(store).CreateUser(context.Background(), CreateUserParams{
		Username:      "Ana",
		Email:         ptr("Ana@Example.com"),
		Password:      "Secret-1",
		IsServerAdmin: true,
	})

	require.NoError(t, err)
	assert.Equal(t, store.createdUser, user)
	assert.Equal(t, "ana", user.Username)
	assert.Equal(t, ptr("ana@example.com"), user.Email)
	assert.True(t, user.IsServerAdmin)
	assert.NotEqual(t, uuid.Nil(), user.ID)

	ok, err := VerifyPassword(context.Background(), user.PasswordHash, "Secret-1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCreateUserRefused(t *testing.T) {
	tests := []struct {
		name      string
		params    CreateUserParams
		storeErr  error
		wantKind  apperr.Kind
		wantCode  apperr.Code
		wantField string
	}{
		{name: "bad username", params: CreateUserParams{Username: "ana!", Password: "Secret-1"}, wantKind: apperr.KindInvalid, wantCode: apperr.CodeInvalidRequest, wantField: "username"},
		{name: "bad email", params: CreateUserParams{Username: "ana", Email: ptr("ana"), Password: "Secret-1"}, wantKind: apperr.KindInvalid, wantCode: apperr.CodeInvalidRequest, wantField: "email"},
		{name: "weak password", params: CreateUserParams{Username: "ana", Password: "secret"}, wantKind: apperr.KindInvalid, wantCode: apperr.CodeInvalidRequest, wantField: "password"},
		{name: "username taken", params: CreateUserParams{Username: "ana", Password: "Secret-1"}, storeErr: domain.ErrUsernameTaken, wantKind: apperr.KindConflict, wantCode: apperr.CodeAlreadyExists, wantField: "username"},
		{name: "email taken", params: CreateUserParams{Username: "ana", Password: "Secret-1"}, storeErr: domain.ErrEmailTaken, wantKind: apperr.KindConflict, wantCode: apperr.CodeAlreadyExists, wantField: "email"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newService(&fakeStore{userErr: tt.storeErr}).CreateUser(context.Background(), tt.params)

			assertAppError(t, err, tt.wantKind, tt.wantCode, tt.wantField)
		})
	}
}

func TestListUsers(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana"}
	after := uuid.NewV7()
	store := &fakeStore{users: []domain.User{ana}}

	users, err := newService(store).ListUsers(context.Background(), &after, 11)

	require.NoError(t, err)
	assert.Equal(t, []domain.User{ana}, users)
	assert.Equal(t, &after, store.listedAfter)
	assert.Equal(t, int32(11), store.listedLimit)
}

func TestUpdateUser(t *testing.T) {
	id := uuid.NewV7()

	t.Run("lowercases the email", func(t *testing.T) {
		store := &fakeStore{}

		user, err := newService(store).UpdateUser(context.Background(), id, domain.UserUpdate{Email: ptr("Ana@Example.com"), Disabled: ptr(true)})

		require.NoError(t, err)
		assert.Equal(t, ptr("ana@example.com"), user.Email)
		assert.Equal(t, id, store.updatedID)
		assert.Equal(t, domain.UserUpdate{Email: ptr("ana@example.com"), Disabled: ptr(true)}, store.updatedUser)
	})

	t.Run("an empty email removes it", func(t *testing.T) {
		store := &fakeStore{}

		_, err := newService(store).UpdateUser(context.Background(), id, domain.UserUpdate{Email: ptr("")})

		require.NoError(t, err)
		assert.Equal(t, ptr(""), store.updatedUser.Email)
	})

	t.Run("a bad email", func(t *testing.T) {
		_, err := newService(&fakeStore{}).UpdateUser(context.Background(), id, domain.UserUpdate{Email: ptr("ana")})

		assertAppError(t, err, apperr.KindInvalid, apperr.CodeInvalidRequest, "email")
	})

	t.Run("the last server admin", func(t *testing.T) {
		_, err := newService(&fakeStore{userErr: domain.ErrLastServerAdmin}).UpdateUser(context.Background(), id, domain.UserUpdate{IsServerAdmin: ptr(false)})

		assertAppError(t, err, apperr.KindConflict, apperr.CodeLastServerAdmin, "")
	})

	t.Run("no user", func(t *testing.T) {
		_, err := newService(&fakeStore{userErr: domain.ErrUserNotFound}).UpdateUser(context.Background(), id, domain.UserUpdate{Disabled: ptr(true)})

		assertAppError(t, err, apperr.KindNotFound, apperr.CodeNotFound, "")
	})

	t.Run("a store failure", func(t *testing.T) {
		_, err := newService(&fakeStore{userErr: errors.New("connection refused")}).UpdateUser(context.Background(), id, domain.UserUpdate{Disabled: ptr(true)})

		assert.EqualError(t, err, "update user: connection refused")
	})
}

func TestResetPassword(t *testing.T) {
	id := uuid.NewV7()

	t.Run("sets the hash", func(t *testing.T) {
		store := &fakeStore{}

		require.NoError(t, newService(store).ResetPassword(context.Background(), id, "Secret-1"))
		assert.Equal(t, id, store.resetID)

		ok, err := VerifyPassword(context.Background(), store.resetHash, "Secret-1")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("a weak password", func(t *testing.T) {
		store := &fakeStore{}

		err := newService(store).ResetPassword(context.Background(), id, "secret")

		assertAppError(t, err, apperr.KindInvalid, apperr.CodeInvalidRequest, "newPassword")
		assert.Empty(t, store.resetHash)
	})

	t.Run("no user", func(t *testing.T) {
		err := newService(&fakeStore{userErr: domain.ErrUserNotFound}).ResetPassword(context.Background(), id, "Secret-1")

		assertAppError(t, err, apperr.KindNotFound, apperr.CodeNotFound, "")
	})
}
