package step

import (
	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
	"github.com/runcoil/runcoil/sdks/go/pkg/retry"
)

// Option configures one Step in Do.
type Option func(*cfg)

// cfg holds what the options of one Step set.
type cfg struct {
	retry *retry.Policy
}

// Retry sets the Step's own retry policy, in place of the Task's.
func Retry(p retry.Policy) Option {
	return func(c *cfg) { c.retry = &p }
}

func retryPolicyToRunstate(p retry.Policy) *runstate.RetryPolicy {
	return &runstate.RetryPolicy{MaxAttempts: p.MaxAttempts, Delays: p.Delays}
}
