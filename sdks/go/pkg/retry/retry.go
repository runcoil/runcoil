// Package retry says how a failed Step is tried again: the Policy a Task or a Step sets,
// and the marks an error carries to fail at once or to choose the wait.
package retry

import (
	"fmt"
	"time"

	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

// MaxAttempts is the most attempts a Policy may ask for, the first one included.
const MaxAttempts = 30

// maxDelays is the most delays a Policy may list.
const maxDelays = 32

// Policy says how often and how late a failed Step is tried again. Each retry is a new
// attempt of the Run: the Steps already done replay, and the failed Step runs again. A
// zero field takes its value from the next policy in line: the Step's (step.Retry), then
// the Task's (runcoil.Retry), then the server's default (4 attempts, 5s, 10s and 30s
// apart).
type Policy struct {
	// MaxAttempts counts every try of the Step, the first one included. At most 30.
	MaxAttempts int
	// Delays are the waits before each retry. The last one repeats. At most 32.
	Delays []time.Duration
}

// Validate reports whether the server accepts the policy.
func (p Policy) Validate() error {
	if p.MaxAttempts < 0 || p.MaxAttempts > MaxAttempts {
		return fmt.Errorf("max attempts must be between 0 and %d, got %d", MaxAttempts, p.MaxAttempts)
	}

	if len(p.Delays) > maxDelays {
		return fmt.Errorf("at most %d delays, got %d", maxDelays, len(p.Delays))
	}

	for _, d := range p.Delays {
		if d < 0 {
			return fmt.Errorf("delays must not be negative, got %s", d)
		}
	}

	return nil
}

// Exponential returns n delays that start at initial and grow by factor each time, each
// at most limit: Exponential(time.Second, 2, time.Minute, 5) is 1s, 2s, 4s, 8s and 16s.
// Use it for Policy.Delays; a policy holds at most 32 delays, and the last one repeats.
func Exponential(initial time.Duration, factor float64, limit time.Duration, n int) []time.Duration {
	delays := make([]time.Duration, 0, max(n, 0))
	d := float64(initial)
	for range n {
		delays = append(delays, min(time.Duration(d), limit))
		d *= factor
	}

	return delays
}

// NonRetriable marks err so that the Step that returns it is not tried again: the Step
// fails for good at once. It returns nil when err is nil. errors.Is and errors.As still
// see err.
func NonRetriable(err error) error {
	if err == nil {
		return nil
	}

	return &runstate.NonRetriableError{Err: err}
}

// After marks err so that the next try of the Step that returns it waits for d, in place
// of the policy's delay. It does not add tries: a Step out of tries still fails. It
// returns nil when err is nil. errors.Is and errors.As still see err.
func After(err error, d time.Duration) error {
	if err == nil {
		return nil
	}

	return &runstate.RetryAfterError{Err: err, After: max(d, 0)}
}
