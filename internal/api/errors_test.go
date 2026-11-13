package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/apierr"
)

func TestDashboardCodesAreInTheSpec(t *testing.T) {
	for _, code := range []apierr.Code{apierr.CodeInvalidRequest, apierr.CodeUnauthenticated, apierr.CodePermissionDenied} {
		assert.True(t, gen.ErrorCode(strings.ToLower(string(code))).Valid(), code)
	}
}

func TestHandleResponseError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{
			name:       "api error",
			err:        apierr.Unauthenticated(apierr.CodeUnauthenticated, "not signed in"),
			wantStatus: http.StatusUnauthorized,
			wantBody:   `{"code": "unauthenticated", "message": "not signed in"}`,
		},
		{
			name:       "code not in the spec",
			err:        apierr.NotFound(apierr.CodeRunNotFound, "run not found"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"code": "internal", "message": "internal error"}`,
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
