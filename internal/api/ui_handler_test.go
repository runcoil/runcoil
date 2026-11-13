package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

func TestUIHandler(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":         {Data: []byte("<html>app</html>")},
		"assets/app-1a2b.js": {Data: []byte("console.log(1)")},
		"favicon.svg":        {Data: []byte("<svg/>")},
	}

	tests := []struct {
		name      string
		method    string
		path      string
		wantCode  int
		wantBody  string
		wantCache string
	}{
		{name: "root", method: http.MethodGet, path: "/", wantCode: http.StatusOK, wantBody: "<html>app</html>", wantCache: "no-cache"},
		{name: "client route", method: http.MethodGet, path: "/projects/1/runs", wantCode: http.StatusOK, wantBody: "<html>app</html>", wantCache: "no-cache"},
		{name: "asset", method: http.MethodGet, path: "/assets/app-1a2b.js", wantCode: http.StatusOK, wantBody: "console.log(1)", wantCache: "public, max-age=31536000, immutable"},
		{name: "public file", method: http.MethodGet, path: "/favicon.svg", wantCode: http.StatusOK, wantBody: "<svg/>"},
		{name: "index by name", method: http.MethodGet, path: "/index.html", wantCode: http.StatusOK, wantBody: "<html>app</html>", wantCache: "no-cache"},
		{name: "missing asset", method: http.MethodGet, path: "/assets/app-old.js", wantCode: http.StatusNotFound},
		{name: "write", method: http.MethodPost, path: "/", wantCode: http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			uiHandler(dist).ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			assert.Equal(t, tt.wantCode, rec.Code)
			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, rec.Body.String())
			}

			assert.Equal(t, tt.wantCache, rec.Header().Get("Cache-Control"))
			if tt.wantCache == "no-cache" {
				assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
			}
		})
	}
}

func TestUIHandlerNotBuilt(t *testing.T) {
	rec := httptest.NewRecorder()
	uiHandler(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "task ui:build")
}

func TestAPIPathsAreNotTheUI(t *testing.T) {
	h := newAPIServer(t, &fakeAccountService{user: testUser})

	rec := do(t, h, call{method: http.MethodGet, path: "/api/v1/nothing"})

	assert.Equal(t, http.StatusNotFound, rec.Code)
	code, _ := decodeError(t, rec)
	assert.Equal(t, "not_found", code)
}
