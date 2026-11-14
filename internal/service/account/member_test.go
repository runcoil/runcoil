package account

import (
	"context"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apperr"
	"github.com/runcoil/runcoil/internal/domain"
)

func TestListMembers(t *testing.T) {
	members := []domain.Member{{UserID: uuid.NewV7(), Username: "ana", Role: domain.RoleViewer}}

	got, err := newService(&fakeStore{members: members}).ListMembers(context.Background(), uuid.NewV7())

	require.NoError(t, err)
	assert.Equal(t, members, got)
}

func TestAddMember(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana", Email: ptr("ana@example.com")}

	t.Run("by username, in any case", func(t *testing.T) {
		store := &fakeStore{users: []domain.User{ana}}

		member, err := newService(store).AddMember(context.Background(), uuid.NewV7(), "Ana", domain.RoleEditor)

		require.NoError(t, err)
		assert.Equal(t, domain.Member{UserID: ana.ID, Username: "ana", Email: ana.Email, Role: domain.RoleEditor}, member)
		assert.Equal(t, ana.ID, store.addedMember)
		assert.Equal(t, domain.RoleEditor, store.addedRole)
	})

	t.Run("no such username", func(t *testing.T) {
		_, err := newService(&fakeStore{}).AddMember(context.Background(), uuid.NewV7(), "bo", domain.RoleEditor)

		assertAppError(t, err, apperr.KindInvalid, apperr.CodeInvalidRequest, "username")
	})

	t.Run("already a member", func(t *testing.T) {
		store := &fakeStore{users: []domain.User{ana}, memberErr: domain.ErrMemberExists}

		_, err := newService(store).AddMember(context.Background(), uuid.NewV7(), "ana", domain.RoleEditor)

		assertAppError(t, err, apperr.KindConflict, apperr.CodeAlreadyExists, "username")
	})
}

func TestUpdateMember(t *testing.T) {
	userID := uuid.NewV7()

	member, err := newService(&fakeStore{}).UpdateMember(context.Background(), uuid.NewV7(), userID, domain.RoleAdmin)
	require.NoError(t, err)
	assert.Equal(t, domain.Member{UserID: userID, Role: domain.RoleAdmin}, member)

	_, err = newService(&fakeStore{memberErr: domain.ErrMemberNotFound}).UpdateMember(context.Background(), uuid.NewV7(), userID, domain.RoleAdmin)
	assertAppError(t, err, apperr.KindNotFound, apperr.CodeNotFound, "")
}

func TestRemoveMember(t *testing.T) {
	require.NoError(t, newService(&fakeStore{}).RemoveMember(context.Background(), uuid.NewV7(), uuid.NewV7()))

	err := newService(&fakeStore{memberErr: domain.ErrMemberNotFound}).RemoveMember(context.Background(), uuid.NewV7(), uuid.NewV7())
	assertAppError(t, err, apperr.KindNotFound, apperr.CodeNotFound, "")
}
