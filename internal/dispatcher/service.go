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

// Config is the timing of the dispatcher and of the worker sessions it holds.
type Config struct {
	PollInterval      time.Duration
	HeartbeatInterval time.Duration
	WorkerTTL         time.Duration
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.PollInterval, "dispatcher.poll-interval", time.Second, "Longest time between two claims.")
	fs.DurationVar(&c.HeartbeatInterval, "dispatcher.heartbeat-interval", 10*time.Second, "How often a worker sends a heartbeat, and the server a keepalive.")
	fs.DurationVar(&c.WorkerTTL, "dispatcher.worker-ttl", 30*time.Second, "With no heartbeat for longer than this, a worker session ends.")
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.PollInterval <= 0 {
		return errors.New("poll interval must be greater than 0")
	}

	if c.HeartbeatInterval <= 0 {
		return errors.New("heartbeat interval must be greater than 0")
	}

	if c.WorkerTTL <= c.HeartbeatInterval {
		return errors.New("worker ttl must be longer than the heartbeat interval")
	}

	return nil
}

// WorkerStore keeps the worker sessions and claims queued Runs.
type WorkerStore interface {
	Register(ctx context.Context, keyHash []byte, w domain.NewWorker) (domain.Worker, error)
	WorkerBySessionToken(ctx context.Context, tokenHash []byte) (domain.Worker, error)
	AttachStream(ctx context.Context, params domain.AttachStreamParams) ([]domain.Lease, error)
	FlushHeartbeats(ctx context.Context, ids []uuid.UUID, beats []time.Time) error
	BeatDispatcher(ctx context.Context, id uuid.UUID) error
	ClaimRuns(ctx context.Context, params domain.ClaimRunsParams) ([]domain.Assign, error)
}

// RunStore keeps the Runs and their journal.
type RunStore interface {
	CompleteStep(ctx context.Context, params domain.CompleteStepParams) (domain.StepRecord, error)
	FinishRun(ctx context.Context, workerID uuid.UUID, lease domain.Lease, end domain.RunEnd) (bool, error)
}

// Service holds the streams of the connected workers, claims queued Runs for the ones
// with free slots, and flushes their heartbeats.
type Service struct {
	cfg         Config
	logger      *slog.Logger
	workerStore WorkerStore
	runStore    RunStore

	// id is this dispatcher's. Each stream it holds carries it, and a claim checks it,
	// so only the dispatcher holding a worker's stream claims Runs for that worker.
	id uuid.UUID

	mu      sync.Mutex
	streams map[uuid.UUID]*stream

	beatsMu sync.Mutex
	beats   map[uuid.UUID]time.Time

	stopped  chan struct{}
	stopOnce sync.Once
}

// NewService creates the dispatcher. It claims nothing until Run.
func NewService(cfg Config, logger *slog.Logger, workerStore WorkerStore, runStore RunStore) (*Service, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("new dispatcher id: %w", err)
	}

	return &Service{
		cfg:         cfg,
		logger:      logger,
		workerStore: workerStore,
		runStore:    runStore,
		id:          id,
		streams:     make(map[uuid.UUID]*stream),
		beats:       make(map[uuid.UUID]time.Time),
		stopped:     make(chan struct{}),
	}, nil
}

// Run claims, flushes heartbeats and records that this dispatcher is alive until Stop is
// called or ctx ends.
func (s *Service) Run(ctx context.Context) error {
	if err := s.workerStore.BeatDispatcher(ctx, s.id); err != nil {
		return err
	}

	s.logger.InfoContext(ctx, "dispatcher started", slog.String("dispatcher_id", s.id.String()))

	claim := time.NewTicker(s.cfg.PollInterval)
	defer claim.Stop()

	flush := time.NewTicker(s.cfg.WorkerTTL / 3)
	defer flush.Stop()

	beat := time.NewTicker(s.cfg.HeartbeatInterval)
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
			if err := s.workerStore.BeatDispatcher(ctx, s.id); err != nil {
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
		assigns, err := s.workerStore.ClaimRuns(ctx, domain.ClaimRunsParams{
			WorkerID:     c.workerID,
			DispatcherID: s.id,
			StreamID:     c.streamID,
			TaskIDs:      c.taskIDs,
			Slots:        c.free,
		})
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

	if err := s.workerStore.FlushHeartbeats(ctx, ids, times); err != nil {
		s.logger.ErrorContext(ctx, "flush heartbeats failed", slog.Any("error", err))
	}
}
