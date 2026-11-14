package api

import (
	"context"
	"fmt"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/domain"
)

type memberHandler struct {
	accountService AccountService
}

func newMemberHandler(accountService AccountService) *memberHandler {
	return &memberHandler{accountService: accountService}
}

func (h *memberHandler) ListMembers(ctx context.Context, req gen.ListMembersRequestObject) (gen.ListMembersResponseObject, error) {
	res, err := h.accountService.ListMembers(ctx, uuid.UUID(req.ProjectId))
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}

	members := make([]gen.Member, 0, len(res))
	for _, member := range res {
		members = append(members, memberToResponse(member))
	}

	return gen.ListMembers200JSONResponse{Members: members}, nil
}

func (h *memberHandler) AddMember(ctx context.Context, req gen.AddMemberRequestObject) (gen.AddMemberResponseObject, error) {
	res, err := h.accountService.AddMember(ctx, uuid.UUID(req.ProjectId), req.Body.Username, domain.Role(req.Body.Role))
	if err != nil {
		return nil, fmt.Errorf("add member: %w", err)
	}

	return gen.AddMember201JSONResponse(memberToResponse(res)), nil
}

func (h *memberHandler) UpdateMember(ctx context.Context, req gen.UpdateMemberRequestObject) (gen.UpdateMemberResponseObject, error) {
	res, err := h.accountService.UpdateMember(ctx, uuid.UUID(req.ProjectId), uuid.UUID(req.UserId), domain.Role(req.Body.Role))
	if err != nil {
		return nil, fmt.Errorf("update member: %w", err)
	}

	return gen.UpdateMember200JSONResponse(memberToResponse(res)), nil
}

func (h *memberHandler) RemoveMember(ctx context.Context, req gen.RemoveMemberRequestObject) (gen.RemoveMemberResponseObject, error) {
	if err := h.accountService.RemoveMember(ctx, uuid.UUID(req.ProjectId), uuid.UUID(req.UserId)); err != nil {
		return nil, fmt.Errorf("remove member: %w", err)
	}

	return gen.RemoveMember204Response{}, nil
}

func memberToResponse(member domain.Member) gen.Member {
	return gen.Member{
		UserId:   openapi_types.UUID(member.UserID),
		Username: member.Username,
		Email:    member.Email,
		Role:     gen.Role(member.Role),
	}
}
