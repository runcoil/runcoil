package step

import (
	"errors"
	"fmt"
	"time"

	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

// MaxAttempts is the most attempts a RetryPolicy may ask for, the first one included.
const MaxAttempts = 30

// maxDelays is the most delays a RetryPolicy may list.
const maxDelays = 32

// RetryPolicy says how often and how late a failed Step is tried again. Each retry is a
// new attempt of the Run: the Steps already done replay, and the failed Step runs again.
// A zero field takes its value from the next policy in line: the Step's, then the Task's,
// then the server's default (4 attempts, 5s, 10s and 30s apart).
type RetryPolicy struct {
	// MaxAttempts counts every try of the Step, the first one included. At most 30.
	MaxAttempts int
	// Delays are the waits before each retry. The last one repeats. At most 32.
	Delays []time.Duration
}

// Validate reports whether the server accepts the policy.
func (p RetryPolicy) Validate() error {
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

// Option configures one Step in Do.
type Option func(*options)

type options struct {
	retry *RetryPolicy
}

// Retry sets the Step's own retry policy, in place of the Task's.
func Retry(p RetryPolicy) Option {
	return func(o *options) { o.retry = &p }
}

// NonRetriable marks err so that the Step that returns it is not tried again: the Step
// fails for good at once. It returns nil when err is nil. errors.Is and errors.As still
// see err.
func NonRetriable(err error) error {
	if err == nil {
		return nil
	}

	return &nonRetriableError{err: err}
}

// RetryAfter marks err so that the next try of the Step that returns it waits for d, in
// place of the policy's delay. It does not add tries: a Step out of tries still fails. It
// returns nil when err is nil. errors.Is and errors.As still see err.
func RetryAfter(err error, d time.Duration) error {
	if err == nil {
		return nil
	}

	return &retryAfterError{err: err, after: max(d, 0)}
}

type nonRetriableError struct {
	err error
}

func (e *nonRetriableError) Error() string { return e.err.Error() }
func (e *nonRetriableError) Unwrap() error { return e.err }

type retryAfterError struct {
	err   error
	after time.Duration
}

func (e *retryAfterError) Error() string { return e.err.Error() }
func (e *retryAfterError) Unwrap() error { return e.err }

// retryFailure adds to f what err says about retrying.
func retryFailure(f *runstate.Failure, err error) {
	var nr *nonRetriableError
	f.NonRetriable = errors.As(err, &nr)

	var ra *retryAfterError
	if errors.As(err, &ra) {
		after := ra.after
		f.RetryAfter = &after
	}
}

func (p RetryPolicy) runstate() *runstate.RetryPolicy {
	return &runstate.RetryPolicy{MaxAttempts: p.MaxAttempts, Delays: p.Delays}
}
