package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckOrigin(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		origin   string
		wantCode int
	}{
		{name: "write from the same origin", method: http.MethodPost, origin: "http://example.com", wantCode: http.StatusNoContent},
		{name: "write without an origin", method: http.MethodPost, wantCode: http.StatusNoContent},
		{name: "read from another origin", method: http.MethodGet, origin: "https://evil.example", wantCode: http.StatusNoContent},
		{name: "write from another origin", method: http.MethodPost, origin: "https://evil.example", wantCode: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := CheckOrigin()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			r := httptest.NewRequest(tt.method, "http://example.com/api/v1/auth/logout", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)

			assert.Equal(t, tt.wantCode, rec.Code)
			if tt.wantCode == http.StatusForbidden {
				assert.JSONEq(t, `{"code": "permission_denied", "message": "cross-origin request refused"}`, rec.Body.String())
			}
		})
	}
}
