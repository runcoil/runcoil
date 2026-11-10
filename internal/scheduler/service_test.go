package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunStore counts the calls of each job. A job with full set returns a full batch
// every time, as a backlog that never ends would.
type fakeRunStore struct {
	mu          sync.Mutex
	promotes    int
	expires     int
	fullPromote bool
	promoteErr  error
}

func (f *fakeRunStore) PromoteDueDelayed(_ context.Context, limit int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.promotes++
	if f.promoteErr != nil {
		return 0, f.promoteErr
	}

	if f.fullPromote {
		return limit, nil
	}

	return 0, nil
}

func (f *fakeRunStore) ExpireStartDeadlines(context.Context, int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.expires++

	return 0, nil
}

func (f *fakeRunStore) calls() (promotes, expires int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.promotes, f.expires
}

func newService(runStore RunStore) *Service {
	return New(Config{RequeueDelayedRunsInterval: 10 * time.Millisecond, StartDeadlineCheckInterval: 10 * time.Millisecond}, slog.New(slog.DiscardHandler), runStore)
}

// start runs s until the test ends, and returns the channel Run's error arrives on.
func start(t *testing.T, s *Service) <-chan error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	t.Cleanup(s.Stop)

	return done
}

func TestRunCallsEachJobOncePerTick(t *testing.T) {
	store := &fakeRunStore{fullPromote: true}
	s := newService(store)
	start(t, s)

	time.Sleep(105 * time.Millisecond)
	promotes, expires := store.calls()

	assert.LessOrEqual(t, promotes, 11, "a full batch waits for the next tick")
	assert.GreaterOrEqual(t, promotes, 5)
	assert.GreaterOrEqual(t, expires, 5, "a backlog of delayed Runs does not hold up the start deadline check")
}

func TestStopEndsRunDuringABacklog(t *testing.T) {
	store := &fakeRunStore{fullPromote: true}
	s := newService(store)
	done := start(t, s)

	require.Eventually(t, func() bool { p, _ := store.calls(); return p >= 2 }, time.Second, 5*time.Millisecond)

	s.Stop()
	s.Stop()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "Run did not return after Stop")
	}
}

func TestAFailedJobIsTriedAgainAtTheNextTick(t *testing.T) {
	store := &fakeRunStore{promoteErr: errors.New("db down")}
	s := newService(store)
	start(t, s)

	require.Eventually(t, func() bool { p, e := store.calls(); return p >= 3 && e >= 3 }, time.Second, 5*time.Millisecond,
		"a failed requeue is tried at each tick, and the start deadline check still runs")
}

func TestReady(t *testing.T) {
	s := newService(&fakeRunStore{})
	require.Error(t, s.Ready(context.Background()), "not running before Run")

	done := start(t, s)
	require.Eventually(t, func() bool { return s.Ready(context.Background()) == nil }, time.Second, 5*time.Millisecond)

	s.Stop()
	require.NoError(t, <-done)
	assert.Error(t, s.Ready(context.Background()), "not running after Stop")
}

func TestConfigValidate(t *testing.T) {
	assert.NoError(t, (&Config{RequeueDelayedRunsInterval: time.Second, StartDeadlineCheckInterval: time.Second}).Validate())
	assert.EqualError(t, (&Config{StartDeadlineCheckInterval: time.Second}).Validate(), "requeue delayed runs interval must be greater than 0")
	assert.EqualError(t, (&Config{RequeueDelayedRunsInterval: time.Second}).Validate(), "start deadline check interval must be greater than 0")
}
