package dispatcher

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

func TestRegistrationError(t *testing.T) {
	err := registrationError(&domain.RegistrationError{Message: "task is declared twice", TaskName: "a"})

	var apiErr *apierr.Error
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, apierr.CodeInvalidRegistration, apiErr.Code)
	assert.Equal(t, "task is declared twice", apiErr.Message)
	assert.Equal(t, "a", apiErr.Details["task_name"])
}

func TestNewSessionTokenHashesTheToken(t *testing.T) {
	first, firstHash, err := newSessionToken()
	require.NoError(t, err)

	second, _, err := newSessionToken()
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.Len(t, firstHash, 32)
}

// fakeRunStore answers CompleteStep with result and keeps the decider it was given.
type fakeRunStore struct {
	result domain.StepResult
	decide domain.RetryDecider
}

func (f *fakeRunStore) CompleteStep(_ context.Context, _ domain.CompleteStepParams, decide domain.RetryDecider) (domain.StepResult, error) {
	f.decide = decide
	return f.result, nil
}

func (f *fakeRunStore) FinishRun(context.Context, uuid.UUID, domain.Lease, domain.RunEnd) (bool, error) {
	return true, nil
}

func retryService(jitter float64, runStore RunStore) *Service {
	return &Service{
		cfg: Config{Retry: RetryConfig{
			MaxAttempts: 3,
			Delays:      []time.Duration{10 * time.Second, time.Minute},
			Jitter:      jitter,
		}},
		logger:   slog.New(slog.DiscardHandler),
		runStore: runStore,
		streams:  map[uuid.UUID]*stream{},
	}
}

func TestCompleteStepFreesTheSlotOfARetry(t *testing.T) {
	workerID := uuid.New()
	params := domain.CompleteStepParams{WorkerID: workerID, StepID: "charge"}

	tests := []struct {
		name     string
		result   domain.StepResult
		wantFree int
	}{
		{name: "a retry ends the attempt", result: domain.StepResult{Retry: &domain.ScheduledRetry{TryCount: 1}}, wantFree: 1},
		{name: "a record keeps it", result: domain.StepResult{Record: domain.StepRecord{StepID: "charge"}}, wantFree: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := retryService(0, &fakeRunStore{result: tt.result})
			s.streams[workerID] = &stream{id: uuid.New(), workerID: workerID, slots: 1, free: 0}

			got, err := s.CompleteStep(context.Background(), params)
			require.NoError(t, err)
			assert.Equal(t, tt.result, got)
			assert.Equal(t, tt.wantFree, s.streams[workerID].free)
		})
	}
}

func TestCompleteStepRefusesAnInvalidRetry(t *testing.T) {
	store := &fakeRunStore{}
	s := retryService(0, store)

	_, err := s.CompleteStep(context.Background(), domain.CompleteStepParams{
		Retry: domain.StepRetry{Policy: &domain.RetryPolicy{MaxAttempts: 31}},
	})

	var apiErr *apierr.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, apierr.CodeInvalidRequest, apiErr.Code)
	assert.Nil(t, store.decide, "nothing reaches the store")
}

func failed(code domain.FailureCode) domain.StepOutcome {
	return domain.StepOutcome{Failure: &domain.Failure{Code: code, Message: "boom"}}
}

func ptr[T any](v T) *T { return &v }

func TestRetryDecider(t *testing.T) {
	s := retryService(0, nil)
	runPolicy := &domain.RetryPolicy{MaxAttempts: 5, Delays: []time.Duration{time.Second}}

	tests := []struct {
		name      string
		outcome   domain.StepOutcome
		retry     domain.StepRetry
		tries     int
		runPolicy *domain.RetryPolicy
		want      time.Duration
		wantRetry bool
	}{
		{name: "server default", outcome: failed(domain.FailureCodeError), tries: 1, want: 10 * time.Second, wantRetry: true},
		{name: "server default, last delay repeats", outcome: failed(domain.FailureCodeError), tries: 2, want: time.Minute, wantRetry: true},
		{name: "server default, out of attempts", outcome: failed(domain.FailureCodeError), tries: 3},
		{name: "run policy over the default", outcome: failed(domain.FailureCodeError), tries: 4, runPolicy: runPolicy, want: time.Second, wantRetry: true},
		{
			name:      "step policy over the run's, delays from the run",
			outcome:   failed(domain.FailureCodeError),
			retry:     domain.StepRetry{Policy: &domain.RetryPolicy{MaxAttempts: 2}},
			tries:     1,
			runPolicy: runPolicy,
			want:      time.Second,
			wantRetry: true,
		},
		{
			name:      "step policy ends the retries",
			outcome:   failed(domain.FailureCodeError),
			retry:     domain.StepRetry{Policy: &domain.RetryPolicy{MaxAttempts: 2}},
			tries:     2,
			runPolicy: runPolicy,
		},
		{name: "retry after replaces the delay", outcome: failed(domain.FailureCodeError), retry: domain.StepRetry{After: ptr(3 * time.Second)}, tries: 1, want: 3 * time.Second, wantRetry: true},
		{name: "retry after does not add attempts", outcome: failed(domain.FailureCodeError), retry: domain.StepRetry{After: ptr(3 * time.Second)}, tries: 3},
		{name: "non retriable", outcome: failed(domain.FailureCodeError), retry: domain.StepRetry{NonRetriable: true}, tries: 1},
		{name: "panic", outcome: failed(domain.FailureCodePanic), tries: 1},
		{name: "output not encodable", outcome: failed(domain.FailureCodeOutputNotEncodable), tries: 1},
		{name: "success", outcome: domain.StepOutcome{Output: []byte(`1`)}, tries: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, retry := s.retryDecider(tt.outcome, tt.retry)(tt.tries, tt.runPolicy)
			assert.Equal(t, tt.wantRetry, retry)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRetryDeciderJittersOnlyThePolicyDelay(t *testing.T) {
	s := retryService(0.5, nil)

	for range 100 {
		got, retry := s.retryDecider(failed(domain.FailureCodeError), domain.StepRetry{})(1, nil)
		require.True(t, retry)
		assert.InDelta(t, float64(10*time.Second), float64(got), float64(5*time.Second))

		got, retry = s.retryDecider(failed(domain.FailureCodeError), domain.StepRetry{After: ptr(3 * time.Second)})(1, nil)
		require.True(t, retry)
		assert.Equal(t, 3*time.Second, got, "retry_after is not spread")
	}
}

func TestJitterStaysInItsRange(t *testing.T) {
	for range 1000 {
		got := jitter(10*time.Second, 0.2)
		assert.GreaterOrEqual(t, got, 8*time.Second)
		assert.LessOrEqual(t, got, 12*time.Second)
	}

	assert.Equal(t, 10*time.Second, jitter(10*time.Second, 0))
	assert.Zero(t, jitter(0, 0.2))
}
