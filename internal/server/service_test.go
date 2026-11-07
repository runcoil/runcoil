package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/pkg/cmdutil"
)

type fakePinger struct {
	err   error
	calls atomic.Int32
}

func (p *fakePinger) Ping(ctx context.Context) error {
	p.calls.Add(1)

	if _, ok := ctx.Deadline(); !ok {
		return errors.New("ping without a deadline")
	}

	return p.err
}

func newTestService(db Pinger) *Service {
	s := NewService(Config{Addr: "127.0.0.1:0"}, slog.New(slog.DiscardHandler), db)
	s.shutdownDelay = 0

	return s
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	return rec
}

func TestConfig(t *testing.T) {
	var cfg Config
	require.NoError(t, cmdutil.Load("test", &cfg, nil, func(string) (string, bool) { return "", false }))

	assert.Equal(t, ":7070", cfg.Addr)
	assert.NoError(t, cfg.Validate())

	for _, addr := range []string{"localhost:7070", "0.0.0.0:0", "[::1]:80"} {
		assert.NoError(t, (&Config{Addr: addr}).Validate(), addr)
	}
	for _, addr := range []string{"", "7070", "localhost:http", ":70000", ":-1"} {
		assert.Error(t, (&Config{Addr: addr}).Validate(), addr)
	}
}

func TestLivez(t *testing.T) {
	db := &fakePinger{err: errors.New("down")}
	s := newTestService(db)

	rec := get(t, s.server.Handler, "/livez")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, db.calls.Load())
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		shuttingDown bool
		want         int
	}{
		{name: "healthy", want: http.StatusOK},
		{name: "database fails", err: errors.New("down"), want: http.StatusServiceUnavailable},
		{name: "shutting down", shuttingDown: true, want: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestService(&fakePinger{err: tt.err})
			s.shuttingDown.Store(tt.shuttingDown)

			rec := get(t, s.server.Handler, "/readyz")

			assert.Equal(t, tt.want, rec.Code)
		})
	}
}

func TestMetrics(t *testing.T) {
	s := newTestService(&fakePinger{})

	rec := get(t, s.server.Handler, "/metrics")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain"), rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Body.String(), "go_goroutines")
}

func TestMethodNotAllowed(t *testing.T) {
	s := newTestService(&fakePinger{})

	rec := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/livez", nil))

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// start serves s on a free port and returns its base URL and the result of serve.
func start(t *testing.T, s *Service) (string, <-chan error) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- s.serve(listener) }()

	return "http://" + listener.Addr().String(), done
}

func status(t *testing.T, url string) int {
	t.Helper()

	code, err := fetch(url)
	require.NoError(t, err)

	return code
}

func fetch(url string) (int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

func TestRunAndStop(t *testing.T) {
	s := newTestService(&fakePinger{})
	s.shutdownDelay = 300 * time.Millisecond
	base, done := start(t, s)

	assert.Equal(t, http.StatusOK, status(t, base+"/readyz"))

	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()

	require.Eventually(t, func() bool {
		code, err := fetch(base + "/readyz")
		return err == nil && code == http.StatusServiceUnavailable
	}, time.Second, 10*time.Millisecond)

	<-stopped
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "serve did not return after Stop")
	}

	_, err := http.Get(base + "/livez")
	assert.Error(t, err)
}

func TestUnencryptedHTTP2(t *testing.T) {
	s := newTestService(&fakePinger{})
	base, done := start(t, s)
	defer func() {
		s.Stop()
		require.NoError(t, <-done)
	}()

	var protocols http.Protocols
	protocols.SetUnencryptedHTTP2(true)
	client := &http.Client{Transport: &http.Transport{Protocols: &protocols}}

	resp, err := client.Get(base + "/livez")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 2, resp.ProtoMajor)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRunListenError(t *testing.T) {
	s := newTestService(&fakePinger{})
	s.cfg.Addr = "256.0.0.1:0"

	assert.ErrorContains(t, s.Run(context.Background()), "listen")
	s.Stop()
}

func TestStopBeforeRun(t *testing.T) {
	s := newTestService(&fakePinger{})
	s.Stop()

	assert.NoError(t, s.Run(context.Background()))
}
