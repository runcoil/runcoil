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

// batchSize is how many Runs one tick of a job moves: 500 a second per server with the
// default poll interval. More is left for the next tick.
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

// RunStore moves Runs on time.
type RunStore interface {
	PromoteDueDelayed(ctx context.Context, batchSize int) (int, error)
	ExpireStartDeadlines(ctx context.Context, batchSize int) (int, error)
}

// Service does what is due at a time: it moves Runs whose wait in run_delayed is over to
// the queue, and fails queued Runs past their start deadline. Each batch locks its rows
// with SKIP LOCKED, so several servers can run one each.
type Service struct {
	cfg      Config
	logger   *slog.Logger
	runStore RunStore

	running  atomic.Bool
	stopped  chan struct{}
	stopOnce sync.Once
}

// New creates the scheduler. It does nothing until Run.
func New(cfg Config, logger *slog.Logger, runStore RunStore) *Service {
	return &Service{
		cfg:      cfg,
		logger:   logger,
		runStore: runStore,
		stopped:  make(chan struct{}),
	}
}

// Run moves due Runs and expires start deadlines until Stop is called or ctx ends. Each
// job runs one batch per tick, so Stop is seen within one batch, and a backlog in one job
// never holds up the other.
func (s *Service) Run(ctx context.Context) error {
	s.running.Store(true)
	defer s.running.Store(false)

	promote := time.NewTicker(s.cfg.PollInterval)
	defer promote.Stop()

	expire := time.NewTicker(s.cfg.PollInterval)
	defer expire.Stop()

	for {
		select {
		case <-s.stopped:
			return nil
		case <-ctx.Done():
			return nil
		case <-promote.C:
			s.promoteDueDelayed(ctx)
		case <-expire.C:
			s.expireStartDeadlines(ctx)
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

// promoteDueDelayed moves one batch of Runs whose wait is over to the queue. A failed batch
// is tried again at the next tick.
func (s *Service) promoteDueDelayed(ctx context.Context) {
	n, err := s.runStore.PromoteDueDelayed(ctx, batchSize)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "promote due delayed runs failed", slog.Any("error", err))
		}

		return
	}

	if n > 0 {
		s.logger.DebugContext(ctx, "delayed runs promoted", slog.Int("runs", n))
	}
}

// expireStartDeadlines fails one batch of queued Runs past their start deadline. A failed
// batch is tried again at the next tick.
func (s *Service) expireStartDeadlines(ctx context.Context) {
	n, err := s.runStore.ExpireStartDeadlines(ctx, batchSize)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "expire start deadlines failed", slog.Any("error", err))
		}

		return
	}

	if n > 0 {
		s.logger.InfoContext(ctx, "runs failed past their start deadline", slog.Int("runs", n))
	}
}
