package swagger

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegister(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, []byte("openapi: 3.1.0\n"))

	tests := []struct {
		path        string
		wantType    string
		wantContent string
	}{
		{path: "/api/docs", wantType: "text/html; charset=utf-8", wantContent: `url: "/api/docs/openapi.yaml"`},
		{path: "/api/docs/openapi.yaml", wantType: "application/yaml", wantContent: "openapi: 3.1.0\n"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.wantType, rec.Header().Get("Content-Type"))
			assert.Contains(t, rec.Body.String(), tt.wantContent)
		})
	}
}
