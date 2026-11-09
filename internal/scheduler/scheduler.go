package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/pflag"
)

// batchSize is how many Runs one statement moves. A sweep repeats while statements come
// back full.
const batchSize = 500

// Config is the timing of the scheduler.
type Config struct {
	PollInterval time.Duration
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&c.PollInterval, "scheduler.poll-interval", time.Second,
		"How often the scheduler moves Runs whose wait is over and fails Runs past their start deadline.")
}

// Validate checks the configuration.
func (c *Config) Validate() error {
	if c.PollInterval <= 0 {
		return errors.New("poll interval must be greater than 0")
	}

	return nil
}

// Store moves Runs on time.
type Store interface {
	PromoteDueDelayed(ctx context.Context, batchSize int) (int, error)
	ExpireStartDeadlines(ctx context.Context, batchSize int) (int, error)
}

// Service does what is due at a time: it moves Runs whose wait in run_delayed is over to
// the queue, and fails queued Runs past their start deadline. Each sweep locks its rows
// with SKIP LOCKED, so several servers can run one each.
type Service struct {
	cfg    Config
	logger *slog.Logger
	store  Store

	running  atomic.Bool
	stopped  chan struct{}
	stopOnce sync.Once
}

// New creates the scheduler. It does nothing until Run.
func New(cfg Config, logger *slog.Logger, store Store) *Service {
	return &Service{
		cfg:     cfg,
		logger:  logger,
		store:   store,
		stopped: make(chan struct{}),
	}
}

// Run sweeps every poll interval until Stop is called or ctx ends.
func (s *Service) Run(ctx context.Context) error {
	s.running.Store(true)
	defer s.running.Store(false)

	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		s.sweep(ctx)

		select {
		case <-s.stopped:
			return nil
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Stop ends Run. It is safe to call more than once, and before Run.
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

// Ready reports whether the scheduler runs.
func (s *Service) Ready(_ context.Context) error {
	if !s.running.Load() {
		return errors.New("not running")
	}

	return nil
}

// sweep runs each job until it has nothing left that is due. A failed job is logged and
// tried again at the next sweep.
func (s *Service) sweep(ctx context.Context) {
	s.drain(ctx, "promote due delayed runs", s.store.PromoteDueDelayed)
	s.drain(ctx, "expire start deadlines", s.store.ExpireStartDeadlines)
}

// drain calls job until a call moves less than a full batch.
func (s *Service) drain(ctx context.Context, name string, job func(context.Context, int) (int, error)) {
	total := 0
	for {
		n, err := job(ctx, batchSize)
		if err != nil {
			if ctx.Err() == nil {
				s.logger.ErrorContext(ctx, "scheduler job failed",
					slog.String("job", name),
					slog.Any("error", err),
				)
			}

			return
		}

		total += n
		if n < batchSize {
			break
		}
	}

	if total > 0 {
		s.logger.DebugContext(ctx, "scheduler job done",
			slog.String("job", name),
			slog.Int("runs", total),
		)
	}
}
