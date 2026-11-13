package api

import (
	"context"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/domain"
)

type authHandler struct {
	accounts AccountService
}

func (h *authHandler) Login(ctx context.Context, request gen.LoginRequestObject) (gen.LoginResponseObject, error) {
	result, err := h.accounts.Login(ctx, request.Body.Login, request.Body.Password)
	if err != nil {
		return nil, err
	}

	return gen.Login200JSONResponse{
		Body: gen.LoginResponse{
			User:                    toUser(result.User),
			PasswordChangeSuggested: result.PasswordChangeSuggested,
		},
		Headers: gen.Login200ResponseHeaders{
			SetCookie: sessionCookie(result.Token, result.ExpiresAt).String(),
		},
	}, nil
}

func (h *authHandler) Logout(ctx context.Context, _ gen.LogoutRequestObject) (gen.LogoutResponseObject, error) {
	c, _ := callerFrom(ctx)
	if err := h.accounts.Logout(ctx, c.token); err != nil {
		return nil, err
	}

	cleared := sessionCookie("", time.Time{}) //nolint:gosec // Secure is off so the dashboard works over plain HTTP.
	cleared.MaxAge = -1

	return gen.Logout204Response{
		Headers: gen.Logout204ResponseHeaders{SetCookie: cleared.String()},
	}, nil
}

func (h *authHandler) GetMe(ctx context.Context, _ gen.GetMeRequestObject) (gen.GetMeResponseObject, error) {
	c, _ := callerFrom(ctx)

	roles, err := h.accounts.ProjectRoles(ctx, c.user)
	if err != nil {
		return nil, err
	}

	projects := make([]gen.ProjectAccess, 0, len(roles))
	for _, role := range roles {
		projects = append(projects, gen.ProjectAccess{
			Id:          openapi_types.UUID(role.ProjectID),
			Name:        role.ProjectName,
			Role:        gen.Role(role.Role),
			Permissions: rolePermissions[role.Role],
		})
	}

	permissions := []gen.Permission{}
	if c.user.IsServerAdmin {
		permissions = serverAdminPermissions
	}

	return gen.GetMe200JSONResponse{
		User:        toUser(c.user),
		Permissions: permissions,
		Projects:    projects,
	}, nil
}

func (h *authHandler) ChangeMyPassword(ctx context.Context, request gen.ChangeMyPasswordRequestObject) (gen.ChangeMyPasswordResponseObject, error) {
	c, _ := callerFrom(ctx)

	err := h.accounts.ChangePassword(ctx, c.user, c.token, request.Body.CurrentPassword, request.Body.NewPassword)
	if err != nil {
		return nil, err
	}

	return gen.ChangeMyPassword204Response{}, nil
}

func sessionCookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // Secure is off so the dashboard works over plain HTTP.
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func toUser(user domain.User) gen.User {
	return gen.User{
		Id:            openapi_types.UUID(user.ID),
		Username:      user.Username,
		Email:         user.Email,
		IsServerAdmin: user.IsServerAdmin,
	}
}
