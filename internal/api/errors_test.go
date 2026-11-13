package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/runcoil/runcoil/internal/apperr"
)

func TestHandleResponseError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "api error",
			err:        apperr.Unauthenticated(apperr.CodeUnauthenticated, "not signed in"),
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"code": "unauthenticated", "message": "not signed in"}`,
		},
		{
			name:       "api error with a field",
			err:        apperr.Invalid(apperr.CodeInvalidRequest, "current password is wrong").WithDetail("field", "currentPassword"),
			wantStatus: http.StatusBadRequest,
			wantBody:   `{"code": "invalid_request", "message": "current password is wrong", "params": [{"field": "currentPassword", "message": "current password is wrong"}]}`,
		},
		{
			name:       "unexpected error",
			err:        assert.AnError,
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"code": "internal", "message": "internal error"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{logger: slog.New(slog.DiscardHandler)}
			rec := httptest.NewRecorder()

			s.handleResponseError(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil), tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.JSONEq(t, tt.wantBody, rec.Body.String())
		})
	}
}
