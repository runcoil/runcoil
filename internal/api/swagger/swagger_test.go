package swagger

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, []byte("openapi: 3.1.0\n"))

	tests := []struct {
		path        string
		wantType    string
		wantContent string
	}{
		{path: "/docs", wantType: "text/html; charset=utf-8", wantContent: `url: "/docs/openapi.yaml"`},
		{path: "/docs/openapi.yaml", wantType: "application/yaml", wantContent: "openapi: 3.1.0\n"},
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

func TestWithVersion(t *testing.T) {
	spec := []byte("openapi: 3.1.0\ninfo:\n  title: Runcoil\n  version: 1.0.0\npaths: {}\n")

	got, err := WithVersion(spec, "v0.3.0")

	require.NoError(t, err)
	assert.Equal(t, "openapi: 3.1.0\ninfo:\n    title: Runcoil\n    version: v0.3.0\npaths: {}\n", string(got))
}

func TestWithVersionErrors(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want string
	}{
		{name: "empty", spec: "", want: "spec is empty"},
		{name: "no info", spec: "openapi: 3.1.0\n", want: "spec has no info"},
		{name: "no version", spec: "info:\n  title: Runcoil\n", want: "spec has no info.version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := WithVersion([]byte(tt.spec), "v0.3.0")

			assert.EqualError(t, err, tt.want)
		})
	}
}
