package domain

import "time"

// MaxRetryAttempts is the most attempts a retry policy may ask for, the first one
// included.
const MaxRetryAttempts = 30

// Or returns p with each unset field taken from fallback: a zero MaxAttempts and empty
// Delays are unset.
func (p RetryPolicy) Or(fallback RetryPolicy) RetryPolicy {
	if p.MaxAttempts == 0 {
		p.MaxAttempts = fallback.MaxAttempts
	}

	if len(p.Delays) == 0 {
		p.Delays = fallback.Delays
	}

	return p
}

// Delay returns the wait after the try number tries failed, the first try being 1, and
// false when that try was the last one. The last delay repeats; no delay means none.
func (p RetryPolicy) Delay(tries int) (time.Duration, bool) {
	if tries >= p.MaxAttempts {
		return 0, false
	}

	if len(p.Delays) == 0 {
		return 0, true
	}

	return p.Delays[min(tries, len(p.Delays))-1], true
}

// StepRetry is what a worker says about retrying a failed try of a Step.
type StepRetry struct {
	// Policy is the Step's own policy. Nil: the Run's policy.
	Policy *RetryPolicy
	// NonRetriable says the Step must not be tried again.
	NonRetriable bool
	// After replaces the policy's delay before the next try.
	After *time.Duration
}

// RetryDecider decides whether a failed try of a Step is tried again. tries counts the
// tries of the Step, this one included, and runPolicy is the Run's policy, nil when the
// Run has none. It returns the wait before the next try, and false when the failure is
// final.
type RetryDecider func(tries int, runPolicy *RetryPolicy) (time.Duration, bool)

// Retriable reports whether a failure of this code may be tried again. A panic and an
// output that cannot be encoded fail the same way every time.
func (c FailureCode) Retriable() bool {
	return c == FailureCodeError
}

// ScheduledRetry is the next try of a Step whose try failed.
type ScheduledRetry struct {
	NextAttemptAt time.Time
	// TryCount is how many tries of the Step failed so far.
	TryCount int
}

// StepResult is what one try of a Step leads to: its stored record, or a retry.
type StepResult struct {
	Record StepRecord
	Retry  *ScheduledRetry
}
