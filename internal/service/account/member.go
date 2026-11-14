package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"github.com/runcoil/runcoil/internal/domain"
)

// ListMembers returns the members of the project, by username.
func (s *Service) ListMembers(ctx context.Context, projectID uuid.UUID) ([]domain.Member, error) {
	members, err := s.projectStore.ListMembers(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}

	return members, nil
}

// AddMember gives the user with username role in the project.
func (s *Service) AddMember(ctx context.Context, projectID uuid.UUID, username string, role domain.Role) (domain.Member, error) {
	user, err := s.userStore.GetUserByUsername(ctx, strings.ToLower(username))
	if errors.Is(err, domain.ErrUserNotFound) {
		return domain.Member{}, ErrNoSuchUsername
	}

	if err != nil {
		return domain.Member{}, fmt.Errorf("get user: %w", err)
	}

	err = s.projectStore.AddMember(ctx, projectID, user.ID, role)
	if errors.Is(err, domain.ErrMemberExists) {
		return domain.Member{}, ErrMemberExists
	}

	if err != nil {
		return domain.Member{}, fmt.Errorf("add member: %w", err)
	}

	return domain.Member{UserID: user.ID, Username: user.Username, Email: user.Email, Role: role}, nil
}

// UpdateMember gives the member userID role in the project.
func (s *Service) UpdateMember(ctx context.Context, projectID, userID uuid.UUID, role domain.Role) (domain.Member, error) {
	member, err := s.projectStore.UpdateMemberRole(ctx, projectID, userID, role)
	if errors.Is(err, domain.ErrMemberNotFound) {
		return domain.Member{}, ErrMemberNotFound
	}

	if err != nil {
		return domain.Member{}, fmt.Errorf("update member role: %w", err)
	}

	return member, nil
}

// RemoveMember takes userID out of the project.
func (s *Service) RemoveMember(ctx context.Context, projectID, userID uuid.UUID) error {
	err := s.projectStore.RemoveMember(ctx, projectID, userID)
	if errors.Is(err, domain.ErrMemberNotFound) {
		return ErrMemberNotFound
	}

	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}

	return nil
}
