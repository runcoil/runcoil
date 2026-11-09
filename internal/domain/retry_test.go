package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryPolicyOr(t *testing.T) {
	fallback := RetryPolicy{MaxAttempts: 4, Delays: []time.Duration{time.Second}}

	assert.Equal(t, fallback, RetryPolicy{}.Or(fallback))
	assert.Equal(t, RetryPolicy{MaxAttempts: 2, Delays: []time.Duration{time.Second}}, RetryPolicy{MaxAttempts: 2}.Or(fallback))
	assert.Equal(t, RetryPolicy{MaxAttempts: 4, Delays: []time.Duration{time.Minute}},
		RetryPolicy{Delays: []time.Duration{time.Minute}}.Or(fallback))
}

func TestRetryPolicyDelay(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 4, Delays: []time.Duration{time.Second, time.Minute}}

	tests := []struct {
		tries int
		want  time.Duration
		retry bool
	}{
		{tries: 1, want: time.Second, retry: true},
		{tries: 2, want: time.Minute, retry: true},
		{tries: 3, want: time.Minute, retry: true},
		{tries: 4, retry: false},
	}

	for _, tt := range tests {
		got, retry := policy.Delay(tt.tries)
		assert.Equal(t, tt.retry, retry, "tries %d", tt.tries)
		assert.Equal(t, tt.want, got, "tries %d", tt.tries)
	}

	got, retry := RetryPolicy{MaxAttempts: 2}.Delay(1)
	assert.True(t, retry)
	assert.Zero(t, got, "no delays: the next try is due at once")

	_, retry = RetryPolicy{MaxAttempts: 1}.Delay(1)
	assert.False(t, retry, "one attempt: no retry")
}

func TestFailureCodeRetriable(t *testing.T) {
	assert.True(t, FailureCodeError.Retriable())
	assert.False(t, FailureCodePanic.Retriable())
	assert.False(t, FailureCodeOutputNotEncodable.Retriable())
}
