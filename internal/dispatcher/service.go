package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/domain"
)

// shutdownTimeout bounds how long a stop waits for the open calls to return and the last
// heartbeats to be written.
const shutdownTimeout = 10 * time.Second

// Config is the port of the worker protocol, and the timing of the dispatcher and of the
// worker sessions it holds.
type Config struct {
	Port              int
	ClaimInterval     time.Duration
	HeartbeatInterval time.Duration
	WorkerTTL         time.Duration
	// Retry is the policy of a failed Step whose Run and Step set none.
	Retry RetryConfig
}

// RetryConfig is the server's default retry policy. Jitter spreads each delay by up to
// that fraction, either way, so Steps that failed together do not retry together.
type RetryConfig struct {
	MaxAttempts int
	Delays      []time.Duration
	Jitter      float64
}

// Policy returns the default policy.
func (c RetryConfig) Policy() domain.RetryPolicy {
	return domain.RetryPolicy{MaxAttempts: c.MaxAttempts, Delays: c.Delays}
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.Port, "dispatcher.port", 7070, "Port of the worker protocol.")
	fs.DurationVar(&c.ClaimInterval, "dispatcher.claim-interval", time.Second, "How often the dispatcher claims queued Runs for the workers with free slots.")
	fs.DurationVar(&c.HeartbeatInterval, "dispatcher.heartbeat-interval", 10*time.Second, "How often a worker sends a heartbeat, and the server a keepalive.")
	fs.DurationVar(&c.WorkerTTL, "dispatcher.worker-ttl", 30*time.Second, "With no heartbeat for longer than this, a worker session ends.")
	fs.IntVar(&c.Retry.MaxAttempts, "retry.max-attempts", 4, "Attempts of a failed Step, the first one included, when neither the Step nor its Task sets them.")
	fs.DurationSliceVar(&c.Retry.Delays, "retry.delays",
		[]time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second, time.Minute, 3 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour},
		"Delays before each retry when neither the Step nor its Task sets them. The last one repeats.")
	fs.Float64Var(&c.Retry.Jitter, "retry.jitter", 0.2, "Fraction by which each retry delay is spread either way, from 0 to 1.")
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}

	if c.ClaimInterval <= 0 {
		return errors.New("claim interval must be greater than 0")
	}

	if c.HeartbeatInterval <= 0 {
		return errors.New("heartbeat interval must be greater than 0")
	}

	if c.WorkerTTL <= c.HeartbeatInterval {
		return errors.New("worker ttl must be longer than the heartbeat interval")
	}

	if c.Retry.MaxAttempts < 1 || c.Retry.MaxAttempts > domain.MaxRetryAttempts {
		return fmt.Errorf("retry max attempts must be between 1 and %d", domain.MaxRetryAttempts)
	}

	if len(c.Retry.Delays) == 0 || slices.ContainsFunc(c.Retry.Delays, func(d time.Duration) bool { return d < 0 }) {
		return errors.New("retry delays must be one or more durations of 0 or more")
	}

	if c.Retry.Jitter < 0 || c.Retry.Jitter > 1 {
		return errors.New("retry jitter must be between 0 and 1")
	}

	return nil
}

// WorkerStore keeps the worker sessions and claims queued Runs.
type WorkerStore interface {
	Register(ctx context.Context, keyHash []byte, session domain.WorkerSession) (domain.Worker, error)
	WorkerBySessionToken(ctx context.Context, tokenHash []byte) (domain.Worker, error)
	AttachStream(ctx context.Context, params domain.AttachStreamParams) ([]domain.Lease, error)
	FlushHeartbeats(ctx context.Context, ids []uuid.UUID, beats []time.Time) error
	BeatDispatcher(ctx context.Context, id uuid.UUID) error
	ClaimRuns(ctx context.Context, params domain.ClaimRunsParams) ([]domain.Assign, error)
}

// RunStore keeps the Runs and their journal.
type RunStore interface {
	CompleteStep(ctx context.Context, params domain.CompleteStepParams, decide domain.RetryDecider) (domain.StepResult, error)
	FinishRun(ctx context.Context, workerID uuid.UUID, lease domain.Lease, end domain.RunEnd) (bool, error)
}

// Service serves the worker protocol on its own port. It holds the streams of the
// connected workers, claims queued Runs for the ones with free slots, and flushes their
// heartbeats.
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

	serving  atomic.Bool
	stopped  chan struct{}
	stopOnce sync.Once
}

// New creates the dispatcher. It serves and claims nothing until Run.
func New(cfg Config, logger *slog.Logger, workerStore WorkerStore, runStore RunStore) (*Service, error) {
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

// Run serves the worker protocol, claims, flushes heartbeats and records that this
// dispatcher is alive until Stop is called or ctx ends.
func (s *Service) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.cfg.Port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	return s.serve(ctx, listener)
}

func (s *Service) serve(ctx context.Context, listener net.Listener) error {
	if err := s.workerStore.BeatDispatcher(ctx, s.id); err != nil {
		return fmt.Errorf("beat dispatcher: %w", errors.Join(err, listener.Close()))
	}

	srv := s.newHTTPServer()

	served := make(chan error, 1)
	go func() {
		served <- srv.Serve(listener)
	}()

	s.serving.Store(true)
	defer s.serving.Store(false)

	s.logger.InfoContext(ctx, "dispatcher listening",
		slog.String("addr", listener.Addr().String()),
		slog.String("dispatcher_id", s.id.String()),
	)

	err := s.loop(ctx, served)

	s.mu.Lock()
	open := len(s.streams)
	s.mu.Unlock()

	s.logger.InfoContext(ctx, "dispatcher stopping", slog.Int("streams", open))

	// The streams end when stopped is closed, so the shutdown does not wait for them. One
	// drain context bounds the shutdown and the last flush, which runs after the shutdown
	// so it writes the heartbeats that arrived during it.
	s.Stop()

	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()

	if shutdownErr := srv.Shutdown(drainCtx); shutdownErr != nil {
		err = errors.Join(err, fmt.Errorf("shutdown http: %w", errors.Join(shutdownErr, srv.Close())))
	}

	s.flushHeartbeats(drainCtx)

	return err
}

// loop claims, flushes and beats until Stop is called, ctx ends or the server fails.
func (s *Service) loop(ctx context.Context, served <-chan error) error {
	claim := time.NewTicker(s.cfg.ClaimInterval)
	defer claim.Stop()

	flush := time.NewTicker(s.cfg.WorkerTTL / 3)
	defer flush.Stop()

	beat := time.NewTicker(s.cfg.HeartbeatInterval)
	defer beat.Stop()

	for {
		select {
		case <-s.stopped:
			return nil
		case <-ctx.Done():
			return nil
		case err := <-served:
			return fmt.Errorf("serve http: %w", err)
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

// Ready reports whether the dispatcher takes workers: it serves and is not stopping.
func (s *Service) Ready(_ context.Context) error {
	if s.isStopping() {
		return errors.New("shutting down")
	}

	if !s.serving.Load() {
		return errors.New("not serving")
	}

	return nil
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
		if errors.Is(err, domain.ErrStreamFenced) {
			s.logger.InfoContext(ctx, "stream fenced",
				slog.String("worker_id", c.workerID.String()),
				slog.String("stream_id", c.streamID.String()),
			)
			s.end(c.workerID, c.streamID)

			continue
		}

		// The claim may have committed before the error, so the worker reconnects and the
		// reconcile requeues what it was never sent.
		if err != nil {
			s.logger.ErrorContext(ctx, "claim runs failed, ending the stream",
				slog.String("worker_id", c.workerID.String()),
				slog.Any("error", err),
			)
			s.end(c.workerID, c.streamID)

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
		return
	}

	s.logger.DebugContext(ctx, "heartbeats flushed", slog.Int("count", len(ids)))
}
