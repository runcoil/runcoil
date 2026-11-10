package runcoil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect/v2"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/proto/gen/go/runcoil/v1/runcoilv1connect"
	"github.com/runcoil/runcoil/sdks/go/pkg/retry"
)

const (
	defaultSlots = 10
	handlerWait  = 2 * time.Second // how long Shutdown waits for the handlers it cancelled
	// deregisterTimeout bounds the Deregister at the end of Shutdown, so a server that is
	// down holds up the shutdown for no longer.
	deregisterTimeout = 5 * time.Second
)

// WorkerOption configures a Worker in NewWorker.
type WorkerOption func(*workerCfg)

// workerCfg holds what the options set, until NewWorker builds the Worker.
type workerCfg struct {
	slots int
}

// validate checks the options that NewWorker received. Its errors do not name the
// function; NewWorker adds that prefix.
func (cfg *workerCfg) validate() error {
	if cfg.slots < 1 {
		return fmt.Errorf("slots must be at least 1, got %d", cfg.slots)
	}

	if uint64(cfg.slots) > math.MaxUint32 {
		return fmt.Errorf("slots %d is too large", cfg.slots)
	}

	return nil
}

// Worker runs the Tasks of one app in this process. Add the Tasks with NewTask, call
// Start once, and call Shutdown to stop.
type Worker struct {
	client    *Client
	app       string
	slotCount uint32
	logger    *slog.Logger
	rpc       runcoilv1connect.WorkerServiceClient
	conn      *connection

	mu         sync.Mutex
	tasks      []*task
	byName     map[string]*task
	starting   bool
	started    bool
	stopped    bool
	session    string
	hbInterval time.Duration
	attempts   map[string]*attempt // held attempts, by Run ID

	slots   chan struct{}  // one token per running handler
	running sync.WaitGroup // one count per attempt until its goroutine ends

	// reportCtx bounds the sending of results. Shutdown cancels it once its drain is over.
	reportCtx  context.Context
	reportStop context.CancelFunc

	// streamCancel ends the open Listen stream, and sessionEnded tells the listen loop that
	// it ended because the session did.
	streamCancel context.CancelFunc
	sessionEnded atomic.Bool

	listenStop context.CancelFunc
	listenDone chan struct{}
	beatStop   context.CancelFunc
	beatDone   chan struct{}

	shutdownOnce sync.Once
	shutdownErr  error
}

type task struct {
	name    string
	handler Handler
	cfg     taskCfg
}

// TaskOption configures a Task in NewTask.
type TaskOption func(*taskCfg)

// taskCfg holds what the options of a Task set, sent with the Task at Register.
type taskCfg struct {
	retry         *retry.Policy
	startDeadline *time.Duration
}

// validate checks the options against the server's limits. Its errors do not name the
// function; NewTask adds that prefix.
func (c *taskCfg) validate() error {
	if c.retry != nil {
		if err := c.retry.Validate(); err != nil {
			return fmt.Errorf("retry policy: %w", err)
		}
	}

	if c.startDeadline != nil && *c.startDeadline <= 0 {
		return fmt.Errorf("start deadline must be greater than 0, got %s", *c.startDeadline)
	}

	return nil
}

// Retry sets the retry policy of the Task's Steps. A Step's own step.Retry comes
// first, and a zero field takes the server's default. The server keeps the policy of a Run
// from when the Run was created, so a new policy applies to new Runs only.
func Retry(p retry.Policy) TaskOption {
	return func(c *taskCfg) { c.retry = &p }
}

// StartDeadline fails a Run of the Task that has not started within d of becoming ready,
// for example because no worker was free. d must be greater than 0. It applies to the
// first start only, not to retries. Without it a Run waits for a worker as long as it
// takes.
func StartDeadline(d time.Duration) TaskOption {
	return func(c *taskCfg) { c.startDeadline = &d }
}

// NewWorker returns a Worker for app. The client must come from NewClient. The Worker does
// not contact the server until Start. Every process that runs the same Tasks uses the
// same app name, and a Task is named by its app and its name. Options such as Slots
// change the defaults.
func NewWorker(client *Client, app string, opts ...WorkerOption) (*Worker, error) {
	if client == nil || client.transport == nil {
		return nil, errors.New("new worker: client must come from NewClient")
	}

	if app == "" {
		return nil, errors.New("new worker: app is required")
	}

	cfg := workerCfg{slots: defaultSlots}
	for _, opt := range opts {
		opt(&cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("new worker: %w", err)
	}

	w := &Worker{
		client:    client,
		app:       app,
		slotCount: uint32(cfg.slots), //nolint:gosec // validate checks the range.
		logger:    client.logger,
		conn: &connection{
			logger:   client.logger,
			app:      app,
			answered: make(chan struct{}, 1),
		},
		byName:   map[string]*task{},
		attempts: map[string]*attempt{},
		slots:    make(chan struct{}, cfg.slots),
	}
	w.reportCtx, w.reportStop = context.WithCancel(context.Background())
	w.rpc = runcoilv1connect.NewWorkerServiceClient(connect.NewClient(client.transport, w.authenticate))

	return w, nil
}

// Slots sets how many Runs the worker runs at one time. The server never gives the worker
// more. The default is 10.
func Slots(n int) WorkerOption {
	return func(cfg *workerCfg) { cfg.slots = n }
}

// NewTask adds a Task with its handler to the worker. The name must be unique within the
// worker. It fails when the name is empty or taken, when the handler is zero, when an
// option is invalid, or when the worker has started. Call it before Start; Start
// registers the Task with the server. Options such as Retry and StartDeadline set the
// Task's settings.
func (w *Worker) NewTask(name string, h Handler, opts ...TaskOption) error {
	if name == "" {
		return errors.New("new task: name is required")
	}

	if h.call == nil {
		return fmt.Errorf("new task %q: handler is required", name)
	}

	t := &task{name: name, handler: h}
	for _, opt := range opts {
		opt(&t.cfg)
	}

	if err := t.cfg.validate(); err != nil {
		return fmt.Errorf("new task %q: %w", name, err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stopped {
		return fmt.Errorf("new task %q: worker is shut down", name)
	}

	if w.starting || w.started {
		return fmt.Errorf("new task %q: worker has already started", name)
	}

	if _, dup := w.byName[name]; dup {
		return fmt.Errorf("new task %q: task name is already registered", name)
	}

	w.tasks = append(w.tasks, t)
	w.byName[name] = t

	return nil
}

// Start registers the Tasks with the server and then listens for Runs in the background.
// It returns once registration succeeds, and returns the error when it fails. The
// context bounds only the registration: the worker keeps running until Shutdown.
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()

	switch {
	case w.stopped:
		w.mu.Unlock()
		return errors.New("start worker: worker is shut down")
	case w.starting || w.started:
		w.mu.Unlock()
		return errors.New("start worker: worker has already started")
	}

	w.starting = true
	w.mu.Unlock()

	if err := w.register(ctx); err != nil {
		w.mu.Lock()
		w.starting = false
		w.mu.Unlock()

		return fmt.Errorf("start worker: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.starting = false
	if w.stopped {
		return errors.New("start worker: worker was shut down while starting")
	}

	listenCtx, listenStop := context.WithCancel(context.Background())
	beatCtx, beatStop := context.WithCancel(context.Background())

	w.started = true
	w.listenStop, w.listenDone = listenStop, make(chan struct{})
	w.beatStop, w.beatDone = beatStop, make(chan struct{})

	go w.listenLoop(listenCtx, w.listenDone)
	go w.heartbeatLoop(beatCtx, w.beatDone)

	return nil
}

// Shutdown stops the worker. It closes the Listen stream first, so the server sends no
// new Run, and then waits for the running handlers to finish. When drainCtx ends first,
// it cancels the handlers that are still running and waits up to 2 seconds for them to
// return. Their Runs are not reported. Last, it ends the worker's session on the server,
// which puts those Runs back in the queue at once, without counting them as lost. When
// that call fails, the server takes them back after the worker TTL. A Step function must
// return promptly when its context is cancelled.
//
// Shutdown returns an error naming the Runs it gave up. Calling it again returns the
// first result.
func (w *Worker) Shutdown(drainCtx context.Context) error {
	w.shutdownOnce.Do(func() {
		w.shutdownErr = w.shutdown(drainCtx)
	})

	return w.shutdownErr
}

func (w *Worker) shutdown(drainCtx context.Context) error {
	defer w.reportStop()

	w.mu.Lock()
	w.stopped = true
	listenStop, listenDone := w.listenStop, w.listenDone
	beatStop, beatDone := w.beatStop, w.beatDone
	w.mu.Unlock()

	if listenStop == nil {
		return nil
	}

	w.mu.Lock()
	running := len(w.attempts)
	w.mu.Unlock()

	w.logger.InfoContext(drainCtx, "worker shutting down", slog.Int("running", running))

	listenStop()
	<-listenDone

	drained := make(chan struct{})
	go func() {
		w.running.Wait()
		close(drained)
	}()

	var gaveUp []string
	select {
	case <-drained:
	case <-drainCtx.Done():
		gaveUp = w.giveUp(drained)
	}

	// Heartbeats keep the session alive until the handlers have returned.
	beatStop()
	<-beatDone
	w.conn.stop()
	w.deregister(drainCtx)

	w.logger.InfoContext(drainCtx, "worker stopped", slog.Int("gave_up", len(gaveUp)))

	if len(gaveUp) > 0 {
		return fmt.Errorf("shutdown: gave up %d runs before their result was sent: %v", len(gaveUp), gaveUp)
	}

	return nil
}

// giveUp ends a drain that has reached its deadline. It cancels every handler that is
// still running and gives the handlers handlerWait to return. Then it cuts short the
// results still being sent, and waits handlerWait again. It returns the Runs whose result
// was not sent, sorted.
func (w *Worker) giveUp(drained <-chan struct{}) []string {
	given := map[string]bool{}

	w.mu.Lock()
	for _, a := range w.attempts {
		if a.end() {
			given[a.runID] = true
		}
	}
	w.mu.Unlock()

	if !wait(drained, handlerWait) {
		w.mu.Lock()
		for _, a := range w.attempts {
			given[a.runID] = true
		}
		w.mu.Unlock()

		w.reportStop()
		wait(drained, handlerWait)
	}

	return slices.Sorted(maps.Keys(given))
}

// wait waits for done for at most d, and reports whether done was closed.
func wait(done <-chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// deregister ends the worker's session at the end of a shutdown. A session the server has
// already ended needs nothing more.
func (w *Worker) deregister(ctx context.Context) {
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deregisterTimeout)
	defer cancel()

	_, err := w.rpc.Deregister(callCtx, &runcoilv1.DeregisterRequest{Reason: "shutdown"})
	switch {
	case err == nil:
		w.logger.DebugContext(ctx, "worker deregistered")
	case errorCode(err) == runcoilv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED:
		w.logger.DebugContext(ctx, "worker session already ended", slog.Any("error", err))
	default:
		w.logger.WarnContext(ctx, "deregister failed; the server takes the given up runs back after the worker ttl",
			slog.Any("error", err))
	}
}
