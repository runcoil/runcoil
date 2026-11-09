package dispatcher

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
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

	tasks := make([]string, 0, len(reg.Tasks))
	for _, t := range reg.Tasks {
		tasks = append(tasks, t.Name)
	}

	s.logger.InfoContext(ctx, "worker registered",
		slog.String("worker_id", worker.ID.String()),
		slog.String("app", worker.App),
		slog.Int("slots", worker.Slots),
		slog.Any("tasks", tasks),
	)

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
		return domain.StepRecord{}, errStaleLease
	}

	if err != nil {
		return domain.StepRecord{}, fmt.Errorf("complete step: %w", err)
	}

	status := domain.StepStatusSucceeded
	if record.Failure != nil {
		status = domain.StepStatusFailed
	}

	s.logger.DebugContext(ctx, "step recorded",
		slog.String("run_id", params.Lease.RunID.String()),
		slog.String("step_id", record.StepID),
		slog.String("status", string(status)),
	)

	return record, nil
}

// CompleteRun ends the attempt with the handler's result.
func (s *Service) CompleteRun(ctx context.Context, worker domain.Worker, lease domain.Lease, result domain.RunResult) error {
	end := result.End()
	ended, err := s.runStore.FinishRun(ctx, worker.ID, lease, end)
	if errors.Is(err, domain.ErrStaleLease) {
		return errStaleLease
	}

	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}

	if !ended {
		s.logger.DebugContext(ctx, "run already ended", slog.String("run_id", lease.RunID.String()))
		return nil
	}

	s.release(worker.ID)

	attrs := []slog.Attr{
		slog.String("run_id", lease.RunID.String()),
		slog.Int64("epoch", lease.Epoch),
		slog.String("worker_id", worker.ID.String()),
		slog.String("status", string(end.Status)),
	}
	if end.Failure != nil {
		attrs = append(attrs, slog.String("error_code", string(end.Failure.Code)))
	}

	s.logger.LogAttrs(ctx, slog.LevelDebug, "run ended", attrs...)

	return nil
}
