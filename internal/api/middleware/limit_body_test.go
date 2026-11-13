package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLimitBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "at the limit", body: strings.Repeat("a", 8)},
		{name: "over the limit", body: strings.Repeat("a", 9), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var readErr error
			h := LimitBody(8)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				_, readErr = io.ReadAll(r.Body)
			}))

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body)))

			if !tt.wantErr {
				require.NoError(t, readErr)
				return
			}

			var maxBytesErr *http.MaxBytesError
			assert.True(t, errors.As(readErr, &maxBytesErr), readErr)
		})
	}
}
