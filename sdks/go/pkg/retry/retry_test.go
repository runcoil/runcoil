package retry

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMarksKeepTheError(t *testing.T) {
	base := errors.New("rate limited")

	assert.ErrorIs(t, NonRetriable(base), base)
	assert.ErrorIs(t, After(base, time.Second), base)
	assert.Equal(t, "rate limited", NonRetriable(base).Error())
	assert.NoError(t, NonRetriable(nil))
	assert.NoError(t, After(nil, time.Second))
}

func TestExponential(t *testing.T) {
	assert.Equal(t,
		[]time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second},
		Exponential(time.Second, 2, 10*time.Second, 5))
	assert.Empty(t, Exponential(time.Second, 2, time.Minute, 0))
	assert.NoError(t, Policy{MaxAttempts: 6, Delays: Exponential(time.Second, 2, time.Minute, 5)}.Validate())
}

func TestPolicyValidate(t *testing.T) {
	assert.NoError(t, Policy{}.Validate())
	assert.EqualError(t, Policy{MaxAttempts: 31}.Validate(), "max attempts must be between 0 and 30, got 31")
	assert.EqualError(t, Policy{Delays: make([]time.Duration, 33)}.Validate(), "at most 32 delays, got 33")
	assert.EqualError(t, Policy{Delays: []time.Duration{-time.Second}}.Validate(), "delays must not be negative, got -1s")
}
