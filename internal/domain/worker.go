package domain

import (
	"errors"
	"fmt"
	"uuid"
)

// MaxSlots is the most Runs one worker process may run at once.
const MaxSlots = 100

// Registration is what one worker process declares at Register.
type Registration struct {
	App   string
	Slots int
	Tasks []TaskDefinition
}

// RegistrationError is a registration the server refuses. TaskName is set when one Task
// is at fault.
type RegistrationError struct {
	Message  string
	TaskName string
}

func (e *RegistrationError) Error() string {
	return e.Message
}

// Validate checks what the request rules do not: the slots, that each Task is declared
// once, and the limits of each Task's retry policy and start deadline.
func (r Registration) Validate() error {
	if r.Slots < 1 || r.Slots > MaxSlots {
		return &RegistrationError{Message: fmt.Sprintf("slots must be between 1 and %d", MaxSlots)}
	}

	if len(r.Tasks) == 0 {
		return &RegistrationError{Message: "a registration needs at least one task"}
	}

	seen := make(map[string]struct{}, len(r.Tasks))
	for _, def := range r.Tasks {
		if _, ok := seen[def.Name]; ok {
			return &RegistrationError{Message: "task is declared twice", TaskName: def.Name}
		}

		seen[def.Name] = struct{}{}

		if def.Retry != nil {
			if err := def.Retry.Validate(); err != nil {
				return &RegistrationError{Message: "retry policy: " + err.Error(), TaskName: def.Name}
			}
		}

		if def.StartDeadline != nil && *def.StartDeadline <= 0 {
			return &RegistrationError{Message: "start deadline must be greater than 0", TaskName: def.Name}
		}

		if def.Schedule != nil {
			if err := def.Schedule.Validate(); err != nil {
				return &RegistrationError{Message: "schedule: " + err.Error(), TaskName: def.Name}
			}
		}
	}

	return nil
}

// AsRegistrationError returns the RegistrationError in err's chain, if any.
func AsRegistrationError(err error) (*RegistrationError, bool) {
	var regErr *RegistrationError
	ok := errors.As(err, &regErr)

	return regErr, ok
}

// WorkerSession is a worker session to store at Register. TaskIDs holds a new ID per
// Task, used only for a Task stored for the first time.
type WorkerSession struct {
	ID           uuid.UUID
	TokenHash    []byte
	Registration Registration
	TaskIDs      []uuid.UUID
}

// NewWorkerSession checks the registration and gives the session and each Task a new ID.
func NewWorkerSession(reg Registration, tokenHash []byte) (WorkerSession, error) {
	if err := reg.Validate(); err != nil {
		return WorkerSession{}, fmt.Errorf("validate registration: %w", err)
	}

	taskIDs := make([]uuid.UUID, 0, len(reg.Tasks))
	for range reg.Tasks {
		taskIDs = append(taskIDs, uuid.NewV7())
	}

	return WorkerSession{ID: uuid.NewV7(), TokenHash: tokenHash, Registration: reg, TaskIDs: taskIDs}, nil
}

// Worker is a live worker session.
type Worker struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	App       string
	TaskIDs   []uuid.UUID
	Slots     int
}

// AttachStreamParams moves a worker to a new stream. Running is what the worker reports
// it is running. Attached gets the reported attempts the worker no longer holds while
// the worker row is still locked, before the commit.
type AttachStreamParams struct {
	WorkerID     uuid.UUID
	DispatcherID uuid.UUID
	StreamID     uuid.UUID
	Running      []Lease
	Attached     func(notHeld []Lease)
}

// ClaimRunsParams claims up to Slots queued Runs of TaskIDs for the worker on the stream.
type ClaimRunsParams struct {
	WorkerID     uuid.UUID
	DispatcherID uuid.UUID
	StreamID     uuid.UUID
	TaskIDs      []uuid.UUID
	Slots        int
}

// DeadWorkers is what one sweep of the reaper did: the sessions it ended, and what became
// of the attempts they held.
type DeadWorkers struct {
	Ended    int
	Requeued int
	// Failed counts the Runs lost more often than the cap, which failed with
	// FailureCodeAttemptLost.
	Failed int
}
