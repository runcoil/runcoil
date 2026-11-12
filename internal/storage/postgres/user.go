package postgres

import (
	"context"
	"fmt"
	"uuid"

	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/internal/storage/postgres/sqlc"
)

// UserStore keeps the dashboard users.
type UserStore struct {
	db      DB
	queries sqlc.Queries
}

// NewUserStore creates a UserStore on db.
func NewUserStore(db DB, queries sqlc.Queries) *UserStore {
	return &UserStore{db: db, queries: queries}
}

// CreateFirstServerAdmin creates admin and the project default in one transaction, only
// while no user exists. It reports whether it created them.
func (s *UserStore) CreateFirstServerAdmin(ctx context.Context, admin domain.User, defaultProjectID uuid.UUID) (bool, error) {
	var created bool
	err := s.db.WithTx(ctx, func(tx DB) error {
		rows, err := s.queries.UserCreateFirstServerAdmin(ctx, tx, sqlc.UserCreateFirstServerAdminParams{
			ID:           admin.ID,
			Username:     admin.Username,
			PasswordHash: admin.PasswordHash,
		})
		if err != nil {
			return fmt.Errorf("create first server admin: %w", err)
		}

		if rows == 0 {
			return nil
		}

		if err := s.queries.ProjectCreateDefault(ctx, tx, defaultProjectID); err != nil {
			return fmt.Errorf("create default project: %w", err)
		}

		created = true

		return nil
	})

	return created, err
}
