package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func decodeLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line), buf.String())

	return line
}

func TestLogRequest(t *testing.T) {
	logger, buf := newTestLogger()
	h := LogRequest(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordUserID(r.Context(), "user-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/v1/things", nil)
	r.RemoteAddr = "10.0.0.1:1234"

	h.ServeHTTP(httptest.NewRecorder(), r)

	line := decodeLogLine(t, buf)
	assert.Equal(t, "INFO", line["level"])
	assert.Equal(t, "request", line["msg"])
	assert.Equal(t, "POST", line["method"])
	assert.Equal(t, "/api/v1/things", line["path"])
	assert.InDelta(t, 201, line["status"], 0)
	assert.InDelta(t, 5, line["bytes"], 0)
	assert.Equal(t, "10.0.0.1:1234", line["remote_addr"])
	assert.Equal(t, "user-1", line["user_id"])
	assert.Contains(t, line, "duration")
	assert.NotContains(t, line, "error")
}

func TestLogRequestWithAnError(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantLevel string
	}{
		{name: "client error", status: http.StatusBadRequest, wantLevel: "INFO"},
		{name: "server error", status: http.StatusInternalServerError, wantLevel: "ERROR"},
		{name: "unavailable", status: http.StatusServiceUnavailable, wantLevel: "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := newTestLogger()
			h := LogRequest(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				RecordError(r.Context(), errors.New("store failed"))
				w.WriteHeader(tt.status)
			}))

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

			line := decodeLogLine(t, buf)
			assert.Equal(t, tt.wantLevel, line["level"])
			assert.Equal(t, "store failed", line["error"])
		})
	}
}

func TestLogRequestDefaultsToOK(t *testing.T) {
	logger, buf := newTestLogger()
	h := LogRequest(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	assert.InDelta(t, 200, decodeLogLine(t, buf)["status"], 0)
}

func TestRecordErrorOutsideARequestDoesNothing(t *testing.T) {
	assert.NotPanics(t, func() {
		RecordError(httptest.NewRequest(http.MethodGet, "/", nil).Context(), errors.New("x"))
	})
}
