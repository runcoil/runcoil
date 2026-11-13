package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecover(t *testing.T) {
	logger, buf := newTestLogger()
	h := Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), LogRequest(logger), Recover())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"code": "internal", "message": "internal error"}`, rec.Body.String())

	line := decodeLogLine(t, buf)
	assert.Equal(t, "ERROR", line["level"])
	assert.Equal(t, "panic: boom", line["error"])
	assert.Contains(t, line["stack"], "recover_test.go")
}

func TestRecoverAbortsAStartedResponse(t *testing.T) {
	logger, buf := newTestLogger()
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("partial"))
		panic("boom")
	}), LogRequest(logger), Recover())

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})

	line := decodeLogLine(t, buf)
	assert.Equal(t, "ERROR", line["level"])
	assert.InDelta(t, http.StatusAccepted, line["status"], 0)
	assert.Equal(t, "panic: boom", line["error"])
	assert.Contains(t, line["stack"], "recover_test.go")
}

func TestRecoverPassesAbortHandlerOn(t *testing.T) {
	h := Recover()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	})
}
