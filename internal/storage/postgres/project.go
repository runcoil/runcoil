package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/internal/storage/postgres/sqlc"
)

// ProjectStore keeps the projects and their API keys.
type ProjectStore struct {
	db DB
	q  *sqlc.Queries
}

// NewProjectStore creates a ProjectStore on db.
func NewProjectStore(db DB) *ProjectStore {
	return &ProjectStore{db: db, q: sqlc.New()}
}

// Bootstrap runs the first start in one transaction: it creates the project when the
// database has none, then adds key to the project named projectName when key is set
// and its hash is not stored yet. It reports whether it created the project and the key.
func (s *ProjectStore) Bootstrap(
	ctx context.Context,
	projectID uuid.UUID,
	projectName string,
	key *domain.BootstrapKey,
) (projectCreated, keyCreated bool, err error) {
	err = s.db.WithTx(ctx, func(tx DB) error {
		if err := s.q.BootstrapLock(ctx, tx); err != nil {
			return fmt.Errorf("lock bootstrap: %w", err)
		}

		exists, err := s.q.ProjectExists(ctx, tx)
		if err != nil {
			return fmt.Errorf("check projects: %w", err)
		}

		if !exists {
			if err := s.q.ProjectCreate(ctx, tx, sqlc.ProjectCreateParams{ID: projectID, Name: projectName}); err != nil {
				return fmt.Errorf("create project: %w", err)
			}

			projectCreated = true
		}

		if key == nil {
			return nil
		}

		keyCreated, err = s.addKey(ctx, tx, projectName, *key)
		return err
	})

	return projectCreated, keyCreated, err
}

func (s *ProjectStore) addKey(ctx context.Context, tx DB, projectName string, key domain.BootstrapKey) (bool, error) {
	exists, err := s.q.ApiKeyExists(ctx, tx, key.Hash)
	if err != nil {
		return false, fmt.Errorf("check api key: %w", err)
	}

	if exists {
		return false, nil
	}

	projectID, err := s.q.ProjectGetByName(ctx, tx, projectName)
	if isNoRowsError(err) {
		return false, fmt.Errorf("project %q: %w", projectName, domain.ErrNotFound)
	}

	if err != nil {
		return false, fmt.Errorf("get project: %w", err)
	}

	err = s.q.ApiKeyCreate(ctx, tx, sqlc.ApiKeyCreateParams{
		ID:        key.ID,
		ProjectID: projectID,
		Name:      "bootstrap",
		KeyHash:   key.Hash,
	})
	if err != nil {
		return false, fmt.Errorf("create api key: %w", err)
	}

	return true, nil
}
