package postgres

import (
	"context"
	"fmt"
	"time"
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

// UserByUsername returns the user with username, or domain.ErrUserNotFound.
func (s *UserStore) UserByUsername(ctx context.Context, username string) (domain.User, error) {
	row, err := s.queries.UserGetByUsername(ctx, s.db, username)
	if isNoRowsError(err) {
		return domain.User{}, domain.ErrUserNotFound
	}

	if err != nil {
		return domain.User{}, fmt.Errorf("get user by username: %w", err)
	}

	return domain.User(row), nil
}

// UserByEmail returns the user with email, or domain.ErrUserNotFound.
func (s *UserStore) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := s.queries.UserGetByEmail(ctx, s.db, email)
	if isNoRowsError(err) {
		return domain.User{}, domain.ErrUserNotFound
	}

	if err != nil {
		return domain.User{}, fmt.Errorf("get user by email: %w", err)
	}

	return domain.User(row), nil
}

// CreateSession creates a session for userID and deletes the user's ended sessions.
func (s *UserStore) CreateSession(ctx context.Context, tokenHash []byte, userID uuid.UUID, maxLifetime, maxInactive time.Duration) error {
	return s.db.WithTx(ctx, func(tx DB) error {
		err := s.queries.SessionDeleteEnded(ctx, tx, sqlc.SessionDeleteEndedParams{
			UserID:      userID,
			MaxInactive: interval(&maxInactive),
		})
		if err != nil {
			return fmt.Errorf("delete ended sessions: %w", err)
		}

		err = s.queries.SessionCreate(ctx, tx, sqlc.SessionCreateParams{
			TokenHash:   tokenHash,
			UserID:      userID,
			MaxLifetime: interval(&maxLifetime),
		})
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}

		return nil
	})
}

// SessionByToken returns the live session of tokenHash, or domain.ErrSessionNotFound when
// the session has ended or its user is disabled.
func (s *UserStore) SessionByToken(ctx context.Context, tokenHash []byte, maxInactive time.Duration) (domain.Session, error) {
	row, err := s.queries.SessionGetUser(ctx, s.db, sqlc.SessionGetUserParams{
		TokenHash:   tokenHash,
		MaxInactive: interval(&maxInactive),
	})
	if isNoRowsError(err) {
		return domain.Session{}, domain.ErrSessionNotFound
	}

	if err != nil {
		return domain.Session{}, fmt.Errorf("get session: %w", err)
	}

	return domain.Session{
		User: domain.User{
			ID:            row.ID,
			Username:      row.Username,
			Email:         row.Email,
			PasswordHash:  row.PasswordHash,
			IsServerAdmin: row.IsServerAdmin,
		},
		TouchDue: row.TouchDue,
	}, nil
}

// TouchSession records activity on the session of tokenHash.
func (s *UserStore) TouchSession(ctx context.Context, tokenHash []byte) error {
	if err := s.queries.SessionTouch(ctx, s.db, tokenHash); err != nil {
		return fmt.Errorf("touch session: %w", err)
	}

	return nil
}

// DeleteSession deletes the session of tokenHash.
func (s *UserStore) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if err := s.queries.SessionDelete(ctx, s.db, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// ChangePassword sets the password hash of userID and deletes the user's sessions other
// than the one of keepTokenHash.
func (s *UserStore) ChangePassword(ctx context.Context, userID uuid.UUID, passwordHash string, keepTokenHash []byte) error {
	return s.db.WithTx(ctx, func(tx DB) error {
		err := s.queries.UserUpdatePassword(ctx, tx, sqlc.UserUpdatePasswordParams{
			ID:           userID,
			PasswordHash: passwordHash,
		})
		if err != nil {
			return fmt.Errorf("update password: %w", err)
		}

		err = s.queries.SessionDeleteOthers(ctx, tx, sqlc.SessionDeleteOthersParams{
			UserID:    userID,
			TokenHash: keepTokenHash,
		})
		if err != nil {
			return fmt.Errorf("delete other sessions: %w", err)
		}

		return nil
	})
}

// ProjectRoles returns the projects userID is a member of, with the role in each.
func (s *UserStore) ProjectRoles(ctx context.Context, userID uuid.UUID) ([]domain.ProjectRole, error) {
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

// Projects returns every project, each with the role admin, as a server admin sees them.
func (s *UserStore) Projects(ctx context.Context) ([]domain.ProjectRole, error) {
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

// MemberRole returns the role of userID in projectID, or domain.ErrMemberNotFound.
func (s *UserStore) MemberRole(ctx context.Context, projectID, userID uuid.UUID) (domain.Role, error) {
	role, err := s.queries.ProjectMemberGetRole(ctx, s.db, sqlc.ProjectMemberGetRoleParams{
		ProjectID: projectID,
		UserID:    userID,
	})
	if isNoRowsError(err) {
		return "", domain.ErrMemberNotFound
	}

	if err != nil {
		return "", fmt.Errorf("get member role: %w", err)
	}

	return domain.Role(role), nil
}
