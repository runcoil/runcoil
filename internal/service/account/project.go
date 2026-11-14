package account

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/runcoil/runcoil/internal/domain"
)

// ListProjects returns the projects the user of identity is a member of, by name. A server
// admin gets every project.
func (s *Service) ListProjects(ctx context.Context, identity domain.Identity) ([]domain.Project, error) {
	user, err := s.userStore.GetUserByID(ctx, identity.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil, ErrNotSignedIn
	}

	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	var projects []domain.Project
	if user.IsServerAdmin {
		projects, err = s.projectStore.ListProjects(ctx)
	} else {
		projects, err = s.projectStore.ListProjectsByMember(ctx, user.ID)
	}

	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	return projects, nil
}

// CreateProject creates a project with no members.
func (s *Service) CreateProject(ctx context.Context, name string) (domain.Project, error) {
	project, err := s.projectStore.CreateProject(ctx, domain.Project{ID: uuid.NewV7(), Name: name})
	if errors.Is(err, domain.ErrProjectNameTaken) {
		return domain.Project{}, ErrProjectNameTaken
	}

	if err != nil {
		return domain.Project{}, fmt.Errorf("create project: %w", err)
	}

	return project, nil
}
