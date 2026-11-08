package http

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/pkg/configutil"
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

type fakeMigrations struct {
	pending bool
	err     error
}

func (m fakeMigrations) HasPending(context.Context) (bool, error) {
	return m.pending, m.err
}

func newTestService(db Pinger) *Service {
	return NewService(Config{Port: 7070}, slog.New(slog.DiscardHandler), db, fakeMigrations{})
}

func get(s *Service, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.newHTTPServer().Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

	return rec
}

func TestConfig(t *testing.T) {
	var cfg Config
	require.NoError(t, configutil.Load("test", &cfg, nil, func(string) (string, bool) { return "", false }))

	assert.Equal(t, 7070, cfg.Port)
	assert.NoError(t, cfg.Validate())

	for _, port := range []int{0, -1, 70000} {
		assert.EqualError(t, (&Config{Port: port}).Validate(), "port must be between 1 and 65535", port)
	}
}

func TestLivez(t *testing.T) {
	db := &fakePinger{err: errors.New("down")}

	rec := get(newTestService(db), "/livez")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, db.calls.Load())
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		migrations fakeMigrations
		stopping   bool
		want       int
		body       string
	}{
		{name: "healthy", want: http.StatusOK, body: "ok"},
		{name: "database fails", err: errors.New("down"), want: http.StatusServiceUnavailable, body: "database unavailable"},
		{name: "migrations pending", migrations: fakeMigrations{pending: true}, want: http.StatusServiceUnavailable, body: "migrations pending"},
		{name: "migration check fails", migrations: fakeMigrations{err: errors.New("down")}, want: http.StatusServiceUnavailable, body: "database unavailable"},
		{name: "stopping", stopping: true, want: http.StatusServiceUnavailable, body: "shutting down"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewService(Config{Port: 7070}, slog.New(slog.DiscardHandler), &fakePinger{err: tt.err}, tt.migrations)
			if tt.stopping {
				s.Stop()
			}

			rec := get(s, "/readyz")

			assert.Equal(t, tt.want, rec.Code)
			assert.Equal(t, tt.body, rec.Body.String())
		})
	}
}

// start serves s on a listener of its own and returns its base URL and the result of serve.
func start(t *testing.T, ctx context.Context, s *Service) (string, <-chan error) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- s.serve(ctx, listener) }()

	return "http://" + listener.Addr().String(), done
}

func wait(t *testing.T, done <-chan error) error {
	t.Helper()

	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		require.FailNow(t, "serve did not return")
		return nil
	}
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

func TestStopShutsTheServerDown(t *testing.T) {
	s := newTestService(&fakePinger{})
	base, done := start(t, context.Background(), s)

	code, err := fetch(base + "/readyz")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, code)

	s.Stop()

	require.NoError(t, wait(t, done))

	_, err = fetch(base + "/livez")
	assert.Error(t, err)
}

func TestContextEndShutsTheServerDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, done := start(t, ctx, newTestService(&fakePinger{}))

	cancel()

	require.NoError(t, wait(t, done))
}

func TestStopBeforeRun(t *testing.T) {
	s := newTestService(&fakePinger{})
	s.Stop()

	_, done := start(t, context.Background(), s)

	assert.NoError(t, wait(t, done))
}

func TestUnencryptedHTTP2(t *testing.T) {
	s := newTestService(&fakePinger{})
	base, done := start(t, context.Background(), s)
	defer func() {
		s.Stop()
		assert.NoError(t, wait(t, done))
	}()

	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	client := &http.Client{Transport: &http.Transport{Protocols: protocols}}

	resp, err := client.Get(base + "/livez")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 2, resp.ProtoMajor)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRunListenError(t *testing.T) {
	s := NewService(Config{Port: 70000}, slog.New(slog.DiscardHandler), &fakePinger{}, fakeMigrations{})

	assert.ErrorContains(t, s.Run(context.Background()), "listen")
}
