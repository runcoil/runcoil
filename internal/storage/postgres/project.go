package postgres

import (
	"context"
	"fmt"
	"uuid"

	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/internal/storage/postgres/sqlc"
)

// ProjectStore keeps the projects and their members.
type ProjectStore struct {
	db      DB
	queries sqlc.Queries
}

// NewProjectStore creates a ProjectStore on db.
func NewProjectStore(db DB, queries sqlc.Queries) *ProjectStore {
	return &ProjectStore{db: db, queries: queries}
}

// ListProjectsWithRoleByUser returns the projects userID is a member of, with the role
// in each.
func (s *ProjectStore) ListProjectsWithRoleByUser(ctx context.Context, userID uuid.UUID) ([]domain.ProjectRole, error) {
	rows, err := s.queries.ProjectMemberListByUser(ctx, s.db, userID)
	if err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}

	roles := make([]domain.ProjectRole, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, domain.ProjectRole{
			ProjectID:   row.ID,
			ProjectName: row.Name,
			Role:        domain.Role(row.Role),
		})
	}

	return roles, nil
}

// ListProjects returns every project, each with the role admin, as a server admin sees
// them.
func (s *ProjectStore) ListProjects(ctx context.Context) ([]domain.ProjectRole, error) {
	rows, err := s.queries.ProjectList(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	roles := make([]domain.ProjectRole, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, domain.ProjectRole{
			ProjectID:   row.ID,
			ProjectName: row.Name,
			Role:        domain.RoleAdmin,
		})
	}

	return roles, nil
}
