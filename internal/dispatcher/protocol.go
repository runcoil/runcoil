package dispatcher

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

// sessionTokenBytes is the size of a session token: 256 random bits.
const sessionTokenBytes = 32

var (
	errInvalidAPIKey       = apierr.Unauthenticated(apierr.CodeUnauthenticated, "invalid api key")
	errInvalidSessionToken = apierr.Unauthenticated(apierr.CodeUnauthenticated, "invalid session token")
	errStaleLease          = apierr.FailedPrecondition(apierr.CodeStaleLease, "lease is no longer this attempt's")
)

// HeartbeatInterval is how often a worker sends a heartbeat.
func (s *Service) HeartbeatInterval() time.Duration {
	return s.worker.HeartbeatInterval
}

// Register stores the Tasks a worker process declares and opens its session. It returns
// the session and its token, which the worker sends on every other call.
func (s *Service) Register(ctx context.Context, apiKey string, reg domain.Registration) (domain.Worker, string, error) {
	if err := validateRegistration(reg); err != nil {
		return domain.Worker{}, "", err
	}

	workerID, err := uuid.NewV7()
	if err != nil {
		return domain.Worker{}, "", fmt.Errorf("new worker id: %w", err)
	}

	taskIDs := make([]uuid.UUID, 0, len(reg.Tasks))
	for range reg.Tasks {
		id, err := uuid.NewV7()
		if err != nil {
			return domain.Worker{}, "", fmt.Errorf("new task id: %w", err)
		}

		taskIDs = append(taskIDs, id)
	}

	token, tokenHash, err := newSessionToken()
	if err != nil {
		return domain.Worker{}, "", err
	}

	keyHash := sha256.Sum256([]byte(apiKey))
	worker, err := s.workers.Register(ctx, keyHash[:], domain.NewWorker{
		ID:           workerID,
		TokenHash:    tokenHash,
		Registration: reg,
		TaskIDs:      taskIDs,
	})
	if isNotFound(err) {
		return domain.Worker{}, "", errInvalidAPIKey
	}

	if err != nil {
		return domain.Worker{}, "", err
	}

	return worker, token, nil
}

func validateRegistration(reg domain.Registration) error {
	if reg.Slots < 1 {
		return apierr.Invalid(apierr.CodeInvalidRegistration, "slots must be at least 1")
	}

	if len(reg.Tasks) == 0 {
		return apierr.Invalid(apierr.CodeInvalidRegistration, "a registration needs at least one task")
	}

	seen := make(map[string]struct{}, len(reg.Tasks))
	for _, def := range reg.Tasks {
		if _, ok := seen[def.Name]; ok {
			return apierr.Invalid(apierr.CodeInvalidRegistration, "task is declared twice").WithDetail("task_name", def.Name)
		}

		seen[def.Name] = struct{}{}
	}

	return nil
}

func newSessionToken() (string, []byte, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("new session token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))

	return token, hash[:], nil
}

// Authenticate returns the live session of the token.
func (s *Service) Authenticate(ctx context.Context, token string) (domain.Worker, error) {
	hash := sha256.Sum256([]byte(token))
	worker, err := s.workers.WorkerBySessionToken(ctx, hash[:])
	if isNotFound(err) {
		return domain.Worker{}, errInvalidSessionToken
	}

	return worker, err
}

// Heartbeat records that the worker process is alive. The next flush writes it.
func (s *Service) Heartbeat(worker domain.Worker) {
	s.beatsMu.Lock()
	defer s.beatsMu.Unlock()

	s.beats[worker.ID] = time.Now()
}

// CompleteStep stores the outcome of one try of a Do Step and returns its record. A
// failed Step is not tried again yet: its failure is final.
func (s *Service) CompleteStep(
	ctx context.Context,
	worker domain.Worker,
	lease domain.Lease,
	stepID string,
	outcome domain.StepOutcome,
) (domain.StepRecord, error) {
	record, err := s.runs.CompleteStep(ctx, worker.ID, lease, stepID, outcome)
	if errors.Is(err, domain.ErrStale) {
		return domain.StepRecord{}, errStaleLease.WithDetail("run_id", lease.RunID.String())
	}

	return record, err
}

// CompleteRun ends the attempt with the handler's result. A failure that carries a
// Step's ID ends the Run as step_failed.
func (s *Service) CompleteRun(ctx context.Context, worker domain.Worker, lease domain.Lease, result domain.RunResult) error {
	end := domain.RunEnd{Status: domain.RunStatusSucceeded, Output: result.Output}
	if result.Failure != nil {
		failure := *result.Failure
		if failure.StepID != "" {
			failure.Code = domain.FailureCodeStepFailed
		}

		end = domain.RunEnd{Status: domain.RunStatusFailed, Failure: &failure}
	}

	ended, err := s.runs.FinishRun(ctx, worker.ID, lease, end)
	if errors.Is(err, domain.ErrStale) {
		return errStaleLease.WithDetail("run_id", lease.RunID.String())
	}

	if err != nil {
		return err
	}

	if ended {
		s.release(worker.ID)
	}

	return nil
}

func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrNotFound)
}
