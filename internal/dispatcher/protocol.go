package dispatcher

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

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
	return s.cfg.HeartbeatInterval
}

// RegisterResult is the session Register opens. The worker sends SessionToken on every
// other call.
type RegisterResult struct {
	Worker       domain.Worker
	SessionToken string
}

// Register stores the Tasks a worker process declares and opens its session.
func (s *Service) Register(ctx context.Context, apiKey string, reg domain.Registration) (RegisterResult, error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("new session token: %w", err)
	}

	session, err := domain.NewWorkerSession(reg, tokenHash)
	if regErr, ok := domain.AsRegistrationError(err); ok {
		return RegisterResult{}, registrationError(regErr)
	}

	if err != nil {
		return RegisterResult{}, fmt.Errorf("new worker session: %w", err)
	}

	keyHash := sha256.Sum256([]byte(apiKey))
	worker, err := s.workerStore.Register(ctx, keyHash[:], session)
	if errors.Is(err, domain.ErrAPIKeyNotFound) {
		return RegisterResult{}, errInvalidAPIKey
	}

	if err != nil {
		return RegisterResult{}, fmt.Errorf("register worker: %w", err)
	}

	return RegisterResult{Worker: worker, SessionToken: token}, nil
}

func registrationError(regErr *domain.RegistrationError) error {
	apiErr := apierr.Invalid(apierr.CodeInvalidRegistration, regErr.Message)
	if regErr.TaskName != "" {
		apiErr = apiErr.WithDetail("task_name", regErr.TaskName)
	}

	return apiErr
}

func newSessionToken() (string, []byte, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("read random bytes: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))

	return token, hash[:], nil
}

// Authenticate returns the live session of the token.
func (s *Service) Authenticate(ctx context.Context, token string) (domain.Worker, error) {
	hash := sha256.Sum256([]byte(token))
	worker, err := s.workerStore.WorkerBySessionToken(ctx, hash[:])
	if errors.Is(err, domain.ErrSessionEnded) {
		return domain.Worker{}, errInvalidSessionToken
	}

	if err != nil {
		return domain.Worker{}, fmt.Errorf("get worker by session token: %w", err)
	}

	return worker, nil
}

// Heartbeat records that the worker process is alive. The next flush writes it.
func (s *Service) Heartbeat(worker domain.Worker) {
	s.beatsMu.Lock()
	defer s.beatsMu.Unlock()

	s.beats[worker.ID] = time.Now()
}

// CompleteStep stores the outcome of one try of a Do Step and returns its record. A
// failed Step is not tried again yet: its failure is final.
func (s *Service) CompleteStep(ctx context.Context, params domain.CompleteStepParams) (domain.StepRecord, error) {
	record, err := s.runStore.CompleteStep(ctx, params)
	if errors.Is(err, domain.ErrStaleLease) {
		return domain.StepRecord{}, errStaleLease.WithDetail("run_id", params.Lease.RunID.String())
	}

	if err != nil {
		return domain.StepRecord{}, fmt.Errorf("complete step: %w", err)
	}

	return record, nil
}

// CompleteRun ends the attempt with the handler's result.
func (s *Service) CompleteRun(ctx context.Context, worker domain.Worker, lease domain.Lease, result domain.RunResult) error {
	ended, err := s.runStore.FinishRun(ctx, worker.ID, lease, result.End())
	if errors.Is(err, domain.ErrStaleLease) {
		return errStaleLease.WithDetail("run_id", lease.RunID.String())
	}

	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}

	if ended {
		s.release(worker.ID)
	}

	return nil
}
