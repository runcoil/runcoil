package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// The outcomes a store reports. Only the engine knows what is missing or stale, so it
// picks the error the client sees.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrStale    = errors.New("stale")
)

// Registration is what one worker process declares at Register.
type Registration struct {
	App   string
	Slots int
	Tasks []TaskDefinition
}

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

// NewWorker is a worker session to store at Register. TaskIDs holds a new ID per Task,
// used only for a Task stored for the first time.
type NewWorker struct {
	ID           uuid.UUID
	TokenHash    []byte
	Registration Registration
	TaskIDs      []uuid.UUID
}

// Worker is a live worker session.
type Worker struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	App       string
	TaskIDs   []uuid.UUID
	Slots     int
}

// Lease names one attempt of one Run.
type Lease struct {
	RunID uuid.UUID
	Epoch int64
}

// Assign is an attempt given to a worker, with what replay needs.
type Assign struct {
	Lease     Lease
	TaskName  string
	Input     []byte
	Steps     []StepRecord
	CreatedAt time.Time
}

// StepKind is the kind of a Step record.
type StepKind string

// The kinds of Step.
const (
	StepKindDo StepKind = "do"
)

// StepRecord is the stored result of one Step. Failure is set when the Step failed for good.
type StepRecord struct {
	StepID  string
	Kind    StepKind
	Output  []byte
	Failure *Failure
}

// FailureCode says why a Step or a Run failed. The values are the names of Failure.Code
// in run.proto, and what runs.error_code and steps.error_code hold.
type FailureCode string

// The failure codes.
const (
	FailureCodeError              FailureCode = "error"
	FailureCodePanic              FailureCode = "panic"
	FailureCodeStepFailed         FailureCode = "step_failed"
	FailureCodeAttemptLost        FailureCode = "attempt_lost"
	FailureCodeStartDeadline      FailureCode = "start_deadline"
	FailureCodeOutputNotEncodable FailureCode = "output_not_encodable"
)

// Failure is why a Step or a Run failed. StepID is set on a Run that failed with a
// Step's final error.
type Failure struct {
	Code    FailureCode
	Message string
	StepID  string
}

// StepOutcome is one try of a Step as the worker reports it: an output, or a failure.
type StepOutcome struct {
	Output  []byte
	Failure *Failure
}

// RunStatus is the status of a Run.
type RunStatus string

// The statuses of a Run.
const (
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
)

// RunResult is how the handler of an attempt ended, as the worker reports it: an output,
// or a failure. A Failure with a StepID is a Step's final error.
type RunResult struct {
	Output  []byte
	Failure *Failure
}

// RunEnd is how an attempt ends a Run.
type RunEnd struct {
	Status  RunStatus
	Output  []byte
	Failure *Failure
}

// StreamMessage is one message for a worker's stream: exactly one field is set.
type StreamMessage struct {
	Assign *Assign
	// LeaseLost names an attempt the worker runs but no longer holds.
	LeaseLost *Lease
	Keepalive bool
}

// BootstrapKey is the API key added to the default project at first start.
type BootstrapKey struct {
	ID   uuid.UUID
	Hash []byte
}
