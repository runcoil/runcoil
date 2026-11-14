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

func TestListProjects(t *testing.T) {
	ana := domain.User{ID: uuid.NewV7(), Username: "ana"}
	admin := domain.User{ID: uuid.NewV7(), Username: "admin", IsServerAdmin: true}
	every := []domain.Project{{ID: uuid.NewV7(), Name: "a"}, {ID: uuid.NewV7(), Name: "b"}}
	mine := every[1:]
	s := newService(&fakeStore{users: []domain.User{ana, admin}, projects: every, mine: mine})

	projects, err := s.ListProjects(context.Background(), identityOf(ana))
	require.NoError(t, err)
	assert.Equal(t, mine, projects)

	projects, err = s.ListProjects(context.Background(), identityOf(admin))
	require.NoError(t, err)
	assert.Equal(t, every, projects)

	_, err = s.ListProjects(context.Background(), domain.Identity{UserID: uuid.NewV7()})
	assertUnauthenticated(t, err)
}

func TestCreateProject(t *testing.T) {
	store := &fakeStore{}

	project, err := newService(store).CreateProject(context.Background(), "billing")

	require.NoError(t, err)
	assert.Equal(t, "billing", project.Name)
	assert.NotEqual(t, uuid.Nil(), project.ID)
	assert.Equal(t, store.createdProject, project)

	_, err = newService(&fakeStore{projectErr: domain.ErrProjectNameTaken}).CreateProject(context.Background(), "billing")
	assertAppError(t, err, apperr.KindConflict, apperr.CodeAlreadyExists, "name")
}
