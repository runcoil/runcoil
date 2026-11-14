package account

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"uuid"

	"github.com/runcoil/runcoil/internal/apperr"
	"github.com/runcoil/runcoil/internal/domain"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// ValidateUsername checks that name is a valid username.
func ValidateUsername(name string) error {
	if !usernamePattern.MatchString(name) {
		return errors.New("must be 1 to 64 characters of a-z, 0-9, '.', '_' and '-'")
	}

	return nil
}

// GetUser returns the user with id.
func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	user, err := s.userStore.GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}

	return user, nil
}

const maxEmailLen = 254

// ValidateEmail checks that email is a plain address such as ana@example.com.
func ValidateEmail(email string) error {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" || len(email) > maxEmailLen {
		return errors.New("must be a plain address such as ana@example.com")
	}

	return nil
}

// CreateUserParams is a new user.
type CreateUserParams struct {
	Username      string
	Email         *string
	Password      string
	IsServerAdmin bool
}

// CreateUser creates a user who can sign in. The username and the email are lowercased.
func (s *Service) CreateUser(ctx context.Context, params CreateUserParams) (domain.User, error) {
	username := strings.ToLower(params.Username)
	if err := ValidateUsername(username); err != nil {
		return domain.User{}, invalidField("username", err)
	}

	email, err := normalizeEmail(params.Email)
	if err != nil {
		return domain.User{}, err
	}

	if err := ValidatePassword(params.Password); err != nil {
		return domain.User{}, invalidField("password", err)
	}

	hash, err := HashPassword(ctx, params.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.userStore.CreateUser(ctx, domain.User{
		ID:            uuid.NewV7(),
		Username:      username,
		Email:         email,
		PasswordHash:  hash,
		IsServerAdmin: params.IsServerAdmin,
	})
	if err != nil {
		return domain.User{}, userStoreError("create user", err)
	}

	return user, nil
}

// ListUsers returns at most limit users by ID, after afterID when it is set.
func (s *Service) ListUsers(ctx context.Context, afterID *uuid.UUID, limit int32) ([]domain.User, error) {
	users, err := s.userStore.ListUsers(ctx, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	return users, nil
}

// UpdateUser changes the email, the server admin role or the disabled state of the user
// with id. Disabling the user ends their sessions.
func (s *Service) UpdateUser(ctx context.Context, id uuid.UUID, update domain.UserUpdate) (domain.User, error) {
	if update.Email != nil && *update.Email != "" {
		email, err := normalizeEmail(update.Email)
		if err != nil {
			return domain.User{}, err
		}

		update.Email = email
	}

	user, err := s.userStore.UpdateUser(ctx, id, update)
	if err != nil {
		return domain.User{}, userStoreError("update user", err)
	}

	return user, nil
}

// ResetPassword sets a new password for the user with id and ends all their sessions.
func (s *Service) ResetPassword(ctx context.Context, id uuid.UUID, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return invalidField("newPassword", err)
	}

	hash, err := HashPassword(ctx, newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.userStore.SetPasswordAndDeleteSessions(ctx, id, hash); err != nil {
		return userStoreError("set password", err)
	}

	return nil
}

func normalizeEmail(email *string) (*string, error) {
	if email == nil {
		return nil, nil
	}

	lower := strings.ToLower(*email)
	if err := ValidateEmail(lower); err != nil {
		return nil, invalidField("email", err)
	}

	return &lower, nil
}

func invalidField(field string, err error) error {
	return apperr.Invalid(apperr.CodeInvalidRequest, field+" "+err.Error()).WithDetail("field", field)
}

func userStoreError(op string, err error) error {
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		return ErrUserNotFound
	case errors.Is(err, domain.ErrUsernameTaken):
		return ErrUsernameTaken
	case errors.Is(err, domain.ErrEmailTaken):
		return ErrEmailTaken
	case errors.Is(err, domain.ErrLastServerAdmin):
		return ErrLastServerAdmin
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}
