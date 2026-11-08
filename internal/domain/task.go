package domain

import "time"

// TaskDefinition is one Task a worker declares. A nil field is unset.
type TaskDefinition struct {
	Name          string
	Retry         *RetryPolicy
	StartDeadline *time.Duration
}

// RetryPolicy says how often and how late a failed Step is tried again.
type RetryPolicy struct {
	// MaxAttempts counts the first attempt. Zero is the server default.
	MaxAttempts int
	// Delays between tries; the last one repeats.
	Delays []time.Duration
}
