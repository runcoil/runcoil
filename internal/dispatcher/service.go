package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/domain"
)

// EngineConfig is the timing of the engine loops.
type EngineConfig struct {
	PollInterval time.Duration
}

// RegisterFlags declares every key with its default and help text.
func (c *EngineConfig) RegisterFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.PollInterval, "engine.poll-interval", time.Second, "Longest time a loop sleeps with no wake-up.")
}

// Validate checks the configuration.
func (c *EngineConfig) Validate() error {
	if c.PollInterval <= 0 {
		return errors.New("poll interval must be greater than 0")
	}

	return nil
}

// WorkerConfig is the timing of worker sessions.
type WorkerConfig struct {
	HeartbeatInterval time.Duration
	TTL               time.Duration
}

// RegisterFlags declares every key with its default and help text.
func (c *WorkerConfig) RegisterFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.HeartbeatInterval, "worker.heartbeat-interval", 10*time.Second, "How often a worker sends a heartbeat, and the server a keepalive.")
	fs.DurationVar(&c.TTL, "worker.ttl", 30*time.Second, "With no heartbeat for longer than this, the worker session ends.")
}

// Validate checks the configuration.
func (c *WorkerConfig) Validate() error {
	if c.HeartbeatInterval <= 0 {
		return errors.New("heartbeat interval must be greater than 0")
	}

	if c.TTL <= c.HeartbeatInterval {
		return errors.New("ttl must be longer than the heartbeat interval")
	}

	return nil
}

// WorkerStore keeps the worker sessions and claims queued Runs.
type WorkerStore interface {
	Register(ctx context.Context, keyHash []byte, w domain.NewWorker) (domain.Worker, error)
	WorkerBySessionToken(ctx context.Context, tokenHash []byte) (domain.Worker, error)
	AttachStream(
		ctx context.Context,
		workerID, dispatcherID, streamID uuid.UUID,
		running []domain.Lease,
		attached func(notHeld []domain.Lease),
	) ([]domain.Lease, error)
	FlushHeartbeats(ctx context.Context, ids []uuid.UUID, beats []time.Time) error
	BeatDispatcher(ctx context.Context, id uuid.UUID) error
	ClaimRuns(
		ctx context.Context,
		workerID, dispatcherID, streamID uuid.UUID,
		taskIDs []uuid.UUID,
		slots int,
	) ([]domain.Assign, error)
}

// RunStore keeps the Runs and their journal.
type RunStore interface {
	CompleteStep(
		ctx context.Context,
		workerID uuid.UUID,
		lease domain.Lease,
		stepID string,
		outcome domain.StepOutcome,
	) (domain.StepRecord, error)
	FinishRun(ctx context.Context, workerID uuid.UUID, lease domain.Lease, end domain.RunEnd) (bool, error)
}

// Service holds the streams of the connected workers, claims queued Runs for the ones
// with free slots, and flushes their heartbeats.
type Service struct {
	engine  EngineConfig
	worker  WorkerConfig
	logger  *slog.Logger
	workers WorkerStore
	runs    RunStore

	// id names this server process in workers.dispatcher_id.
	id uuid.UUID

	mu      sync.Mutex
	streams map[uuid.UUID]*stream

	beatsMu sync.Mutex
	beats   map[uuid.UUID]time.Time

	stopped  chan struct{}
	stopOnce sync.Once
}

// NewService creates the dispatcher. It claims nothing until Run.
func NewService(engine EngineConfig, worker WorkerConfig, logger *slog.Logger, workers WorkerStore, runs RunStore) (*Service, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("new dispatcher id: %w", err)
	}

	return &Service{
		engine:  engine,
		worker:  worker,
		logger:  logger,
		workers: workers,
		runs:    runs,
		id:      id,
		streams: make(map[uuid.UUID]*stream),
		beats:   make(map[uuid.UUID]time.Time),
		stopped: make(chan struct{}),
	}, nil
}

// Run claims, flushes heartbeats and records that this dispatcher is alive until Stop is
// called or ctx ends.
func (s *Service) Run(ctx context.Context) error {
	if err := s.workers.BeatDispatcher(ctx, s.id); err != nil {
		return err
	}

	s.logger.InfoContext(ctx, "dispatcher started", slog.String("dispatcher_id", s.id.String()))

	claim := time.NewTicker(s.engine.PollInterval)
	defer claim.Stop()

	flush := time.NewTicker(s.worker.TTL / 3)
	defer flush.Stop()

	beat := time.NewTicker(s.worker.HeartbeatInterval)
	defer beat.Stop()

	for {
		select {
		case <-s.stopped:
			s.flushHeartbeats(context.WithoutCancel(ctx))
			return nil
		case <-ctx.Done():
			s.flushHeartbeats(context.WithoutCancel(ctx))
			return nil
		case <-claim.C:
			s.claimAll(ctx)
		case <-flush.C:
			s.flushHeartbeats(ctx)
		case <-beat.C:
			if err := s.workers.BeatDispatcher(ctx, s.id); err != nil {
				s.logger.ErrorContext(ctx, "beat dispatcher failed", slog.Any("error", err))
			}
		}
	}
}

// Stop ends the streams and the loops. It is safe to call more than once, and before Run.
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

func (s *Service) isStopping() bool {
	select {
	case <-s.stopped:
		return true
	default:
		return false
	}
}

// claimAll claims queued Runs for every stream with free slots and pushes them.
func (s *Service) claimAll(ctx context.Context) {
	for _, c := range s.claimable() {
		assigns, err := s.workers.ClaimRuns(ctx, c.workerID, s.id, c.streamID, c.taskIDs, c.free)
		if errors.Is(err, domain.ErrStale) {
			s.logger.InfoContext(ctx, "stream fenced",
				slog.String("worker_id", c.workerID.String()),
				slog.String("stream_id", c.streamID.String()),
			)
			s.drop(c.workerID, c.streamID)

			continue
		}

		if err != nil {
			s.logger.ErrorContext(ctx, "claim runs failed",
				slog.String("worker_id", c.workerID.String()),
				slog.Any("error", err),
			)

			continue
		}

		s.push(ctx, c.workerID, c.streamID, assigns)
	}
}

// flushHeartbeats writes the heartbeats received since the last flush.
func (s *Service) flushHeartbeats(ctx context.Context) {
	s.beatsMu.Lock()
	beats := s.beats
	s.beats = make(map[uuid.UUID]time.Time, len(beats))
	s.beatsMu.Unlock()

	if len(beats) == 0 {
		return
	}

	ids := make([]uuid.UUID, 0, len(beats))
	times := make([]time.Time, 0, len(beats))
	for id, at := range beats {
		ids = append(ids, id)
		times = append(times, at)
	}

	if err := s.workers.FlushHeartbeats(ctx, ids, times); err != nil {
		s.logger.ErrorContext(ctx, "flush heartbeats failed", slog.Any("error", err))
	}
}
