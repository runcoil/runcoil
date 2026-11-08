package domain

import "github.com/google/uuid"

// StepKind is what a Step record records.
type StepKind string

// The kinds of Step.
const (
	StepKindDo StepKind = "do"
)

// StepStatus says whether a Step succeeded or failed.
type StepStatus string

// The statuses of a Step record.
const (
	StepStatusSucceeded StepStatus = "succeeded"
	StepStatusFailed    StepStatus = "failed"
)

// StepRecord is the stored result of one Step. Failure is set when the Step failed for good.
type StepRecord struct {
	StepID  string
	Kind    StepKind
	Output  []byte
	Failure *Failure
}

// StepOutcome is one try of a Step as the worker reports it: an output, or a failure.
type StepOutcome struct {
	Output  []byte
	Failure *Failure
}

// Status is the status the outcome is stored with. A failed try is final until Steps
// are retried.
func (o StepOutcome) Status() StepStatus {
	if o.Failure != nil {
		return StepStatusFailed
	}

	return StepStatusSucceeded
}

// CompleteStepParams stores the outcome of one try of a Do Step for the attempt that
// holds Lease.
type CompleteStepParams struct {
	WorkerID uuid.UUID
	Lease    Lease
	StepID   string
	Outcome  StepOutcome
}
