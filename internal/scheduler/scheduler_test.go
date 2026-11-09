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

// fakeStore has a number of due Runs for each job, and records each call.
type fakeStore struct {
	mu       sync.Mutex
	delayed  int
	expired  int
	promotes int
	expires  int
	err      error
}

func take(left *int, limit int) int {
	n := min(*left, limit)
	*left -= n

	return n
}

func (f *fakeStore) PromoteDueDelayed(_ context.Context, limit int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.promotes++
	if f.err != nil {
		return 0, f.err
	}

	return take(&f.delayed, limit), nil
}

func (f *fakeStore) ExpireStartDeadlines(_ context.Context, limit int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.expires++

	return take(&f.expired, limit), nil
}

func newService(store Store) *Service {
	return New(Config{PollInterval: 10 * time.Millisecond}, slog.New(slog.DiscardHandler), store)
}

func TestSweepDrainsFullBatches(t *testing.T) {
	store := &fakeStore{delayed: 2*batchSize + 1, expired: 3}

	newService(store).sweep(context.Background())

	assert.Zero(t, store.delayed)
	assert.Zero(t, store.expired)
	assert.Equal(t, 3, store.promotes, "two full batches, then one that is not full")
	assert.Equal(t, 1, store.expires)
}

func TestSweepGoesOnAfterAFailedJob(t *testing.T) {
	store := &fakeStore{expired: 1, err: errors.New("db down")}

	newService(store).sweep(context.Background())

	assert.Equal(t, 1, store.promotes, "a failed job is not called again in the same sweep")
	assert.Zero(t, store.expired, "the next job still runs")
}

func TestRunSweepsUntilStop(t *testing.T) {
	store := &fakeStore{}
	s := newService(store)
	require.Error(t, s.Ready(context.Background()))

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()

	require.Eventually(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()

		return store.promotes >= 3
	}, time.Second, 5*time.Millisecond, "Run sweeps every poll interval")
	assert.NoError(t, s.Ready(context.Background()))

	s.Stop()
	s.Stop()
	require.NoError(t, <-done)
	assert.Error(t, s.Ready(context.Background()))
}

func TestConfigValidate(t *testing.T) {
	assert.NoError(t, (&Config{PollInterval: time.Second}).Validate())
	assert.EqualError(t, (&Config{}).Validate(), "poll interval must be greater than 0")
}
