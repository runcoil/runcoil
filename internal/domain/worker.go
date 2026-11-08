package domain

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

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

// Validate checks what the request rules cannot: the slots, and that each Task is
// declared once.
func (r Registration) Validate() error {
	if r.Slots < 1 {
		return &RegistrationError{Message: "slots must be at least 1"}
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

	id, err := uuid.NewV7()
	if err != nil {
		return WorkerSession{}, fmt.Errorf("new worker id: %w", err)
	}

	taskIDs := make([]uuid.UUID, 0, len(reg.Tasks))
	for range reg.Tasks {
		taskID, err := uuid.NewV7()
		if err != nil {
			return WorkerSession{}, fmt.Errorf("new task id: %w", err)
		}

		taskIDs = append(taskIDs, taskID)
	}

	return WorkerSession{ID: id, TokenHash: tokenHash, Registration: reg, TaskIDs: taskIDs}, nil
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

// StreamMessage is one message for a worker's stream: exactly one field is set.
type StreamMessage struct {
	Assign *Assign
	// LeaseLost names an attempt the worker runs but no longer holds.
	LeaseLost *Lease
	Keepalive bool
}
