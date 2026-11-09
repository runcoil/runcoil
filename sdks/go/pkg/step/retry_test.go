package step

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryMarksKeepTheError(t *testing.T) {
	base := errors.New("rate limited")

	assert.ErrorIs(t, NonRetriable(base), base)
	assert.ErrorIs(t, RetryAfter(base, time.Second), base)
	assert.Equal(t, "rate limited", NonRetriable(base).Error())
	assert.NoError(t, NonRetriable(nil))
	assert.NoError(t, RetryAfter(nil, time.Second))
}

func TestExponentialDelays(t *testing.T) {
	assert.Equal(t,
		[]time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second},
		ExponentialDelays(time.Second, 2, 10*time.Second, 5))
	assert.Empty(t, ExponentialDelays(time.Second, 2, time.Minute, 0))
	assert.NoError(t, RetryPolicy{MaxAttempts: 6, Delays: ExponentialDelays(time.Second, 2, time.Minute, 5)}.Validate())
}

func TestRetryPolicyValidate(t *testing.T) {
	assert.NoError(t, RetryPolicy{}.Validate())
	assert.EqualError(t, RetryPolicy{MaxAttempts: 31}.Validate(), "max attempts must be between 0 and 30, got 31")
	assert.EqualError(t, RetryPolicy{Delays: make([]time.Duration, 33)}.Validate(), "at most 32 delays, got 33")
	assert.EqualError(t, RetryPolicy{Delays: []time.Duration{-time.Second}}.Validate(), "delays must not be negative, got -1s")
}
