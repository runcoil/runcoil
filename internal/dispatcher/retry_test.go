package dispatcher

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/runcoil/runcoil/internal/domain"
	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
)

func retryService(jitter float64) *Service {
	return &Service{
		cfg: Config{Retry: RetryConfig{
			MaxAttempts: 3,
			Delays:      []time.Duration{10 * time.Second, time.Minute},
			Jitter:      jitter,
		}},
		logger: slog.New(slog.DiscardHandler),
	}
}

func failed(code domain.FailureCode) domain.StepOutcome {
	return domain.StepOutcome{Failure: &domain.Failure{Code: code, Message: "boom"}}
}

func ptr[T any](v T) *T { return &v }

func TestRetryDecider(t *testing.T) {
	s := retryService(0)
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

func TestJitterStaysInItsRange(t *testing.T) {
	for range 1000 {
		got := jitter(10*time.Second, 0.2)
		assert.GreaterOrEqual(t, got, 8*time.Second)
		assert.LessOrEqual(t, got, 12*time.Second)
	}

	assert.Equal(t, 10*time.Second, jitter(10*time.Second, 0))
	assert.Zero(t, jitter(0, 0.2))
}

func TestRetryPolicyFromProto(t *testing.T) {
	assert.Nil(t, retryPolicyFromProto(nil))

	got := retryPolicyFromProto(&runcoilv1.RetryPolicy{
		MaxAttempts: 3,
		Delays:      []*durationpb.Duration{durationpb.New(time.Second), durationpb.New(time.Minute)},
	})
	assert.Equal(t, &domain.RetryPolicy{MaxAttempts: 3, Delays: []time.Duration{time.Second, time.Minute}}, got)
}

func TestStepResultToProto(t *testing.T) {
	at := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

	retry := stepResultToProto(domain.StepResult{Retry: &domain.ScheduledRetry{NextAttemptAt: at, TryCount: 2}})
	assert.Equal(t, timestamppb.New(at).AsTime(), retry.GetRetryScheduled().GetNextAttemptAt().AsTime())
	assert.Equal(t, uint32(2), retry.GetRetryScheduled().GetTryCount())
	assert.Nil(t, retry.GetRecord())

	record := stepResultToProto(domain.StepResult{Record: domain.StepRecord{StepID: "charge", Kind: domain.StepKindDo, Output: []byte(`1`)}})
	assert.Equal(t, "charge", record.GetRecord().GetStepId())
	assert.Nil(t, record.GetRetryScheduled())
}
