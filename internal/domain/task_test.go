package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryPolicyValidate(t *testing.T) {
	assert.NoError(t, RetryPolicy{}.Validate())
	assert.NoError(t, RetryPolicy{MaxAttempts: MaxRetryAttempts, Delays: make([]time.Duration, MaxRetryDelays)}.Validate())

	assert.EqualError(t, RetryPolicy{MaxAttempts: 31}.Validate(), "max attempts must be between 0 and 30")
	assert.EqualError(t, RetryPolicy{MaxAttempts: -1}.Validate(), "max attempts must be between 0 and 30")
	assert.EqualError(t, RetryPolicy{Delays: make([]time.Duration, 33)}.Validate(), "at most 32 delays")
	assert.EqualError(t, RetryPolicy{Delays: []time.Duration{-time.Second}}.Validate(), "delays must not be negative")
}

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
