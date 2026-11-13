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

// GetUserByUsername returns the user with username, or domain.ErrUserNotFound.
func (s *UserStore) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	row, err := s.queries.UserGetByUsername(ctx, s.db, username)
	if isNoRowsError(err) {
		return domain.User{}, domain.ErrUserNotFound
	}

	if err != nil {
		return domain.User{}, fmt.Errorf("get user by username: %w", err)
	}

	return domain.User(row), nil
}

// GetUserByEmail returns the user with email, or domain.ErrUserNotFound.
func (s *UserStore) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := s.queries.UserGetByEmail(ctx, s.db, email)
	if isNoRowsError(err) {
		return domain.User{}, domain.ErrUserNotFound
	}

	if err != nil {
		return domain.User{}, fmt.Errorf("get user by email: %w", err)
	}

	return domain.User(row), nil
}

// UpdatePasswordAndDeleteOtherSessions sets the password hash of userID and deletes the
// user's sessions other than the one of keepTokenHash, in one transaction. It returns
// domain.ErrSessionNotFound when that session has ended, and changes nothing then.
func (s *UserStore) UpdatePasswordAndDeleteOtherSessions(ctx context.Context, userID uuid.UUID, passwordHash string, keepTokenHash []byte) error {
	return s.db.WithTx(ctx, func(tx DB) error {
		// The user row is locked first, so two password changes of one user run one after
		// the other and the second sees that the first ended its session.
		if _, err := s.queries.UserLockForUpdate(ctx, tx, userID); err != nil {
			return fmt.Errorf("lock user: %w", err)
		}

		live, err := s.queries.SessionExistsForUser(ctx, tx, sqlc.SessionExistsForUserParams{
			TokenHash: keepTokenHash,
			UserID:    userID,
		})
		if err != nil {
			return fmt.Errorf("check session: %w", err)
		}

		if !live {
			return domain.ErrSessionNotFound
		}

		err = s.queries.UserUpdatePassword(ctx, tx, sqlc.UserUpdatePasswordParams{
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
