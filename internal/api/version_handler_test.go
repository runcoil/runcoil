package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/api/gen"
	"github.com/runcoil/runcoil/internal/build"
)

func TestGetVersion(t *testing.T) {
	accountService := &fakeAccountService{user: testUser}
	h := newAPIServer(t, accountService)

	rec := do(t, h, call{method: http.MethodGet, path: "/api/v1/version"})

	require.Equal(t, http.StatusOK, rec.Code)

	var res gen.Version
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))

	info := build.Get()
	assert.Equal(t, gen.Version{
		Version:   info.Version,
		Commit:    info.Commit,
		BuildDate: info.BuildDate,
		GoVersion: info.GoVersion,
	}, res)
	assert.Zero(t, accountService.authenticated)
}
