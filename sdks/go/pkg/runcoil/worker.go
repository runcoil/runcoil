package runcoil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/proto/gen/go/runcoil/v1/runcoilv1connect"
	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

const (
	defaultSlots    = 10
	reconnectStart  = 500 * time.Millisecond
	reconnectCap    = 10 * time.Second
	registerTimeout = 10 * time.Second // bounds Register, and every call before the server has answered it
	minCallTimeout  = time.Second      // floor for unary calls after registration
	retryStart      = 250 * time.Millisecond
	retryCap        = 5 * time.Second
	handlerWait     = 2 * time.Second // how long Shutdown waits for the handlers it cancelled
	// silentBeats is how many heartbeat intervals a Listen stream may stay silent before
	// the worker drops it. The server sends a keepalive every interval.
	silentBeats = 3
)

// reminderEvery is how often an outage repeats its warning while it lasts.
var reminderEvery = time.Minute

// WorkerOption configures a Worker in NewWorker.
type WorkerOption func(*workerCfg)

// workerCfg holds what the options set, until NewWorker builds the Worker.
type workerCfg struct {
	app   string
	slots int
}

// validate checks the app and the slots that NewWorker received. Its errors do not name
// the function; NewWorker adds that prefix.
func (cfg *workerCfg) validate() error {
	if cfg.app == "" {
		return errors.New("app is required")
	}

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
}

// NewWorker returns a Worker for the app given by App, which is required. The client must
// come from NewClient. The Worker does not contact the server until Start. Every process
// that runs the same Tasks uses the same app name, and a Task is named by its app and its
// name. Options such as Slots change the defaults.
func NewWorker(client *Client, opts ...WorkerOption) (*Worker, error) {
	if client == nil || client.transport == nil {
		return nil, errors.New("new worker: client must come from NewClient")
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
		app:       cfg.app,
		slotCount: uint32(cfg.slots), //nolint:gosec // validate checks the range.
		logger:    client.logger,
		conn: &connection{
			logger:   client.logger,
			app:      cfg.app,
			answered: make(chan struct{}, 1),
		},
		byName:   map[string]*task{},
		attempts: map[string]*attempt{},
		slots:    make(chan struct{}, cfg.slots),
	}
	w.rpc = runcoilv1connect.NewWorkerServiceClient(connect.NewClient(client.transport, w.authenticate))

	return w, nil
}

// App names the app of the worker. It is required.
func App(name string) WorkerOption {
	return func(cfg *workerCfg) { cfg.app = name }
}

// Slots sets how many Runs the worker runs at one time. The server never gives the worker
// more. The default is 10.
func Slots(n int) WorkerOption {
	return func(cfg *workerCfg) { cfg.slots = n }
}

// NewTask adds a Task with its handler to the worker. The name must be unique within the
// worker. It fails when the name is empty or taken, when the handler is zero, or when the
// worker has started. Call it before Start; Start registers the Task with the server.
func (w *Worker) NewTask(name string, h Handler) error {
	if name == "" {
		return errors.New("new task: name is required")
	}

	if h.call == nil {
		return fmt.Errorf("new task %q: handler is required", name)
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

	t := &task{name: name, handler: h}
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
// return. Their Runs are not reported, and the server takes them back later. A Step
// function must return promptly when its context is cancelled.
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
	w.mu.Lock()
	w.stopped = true
	listenStop, listenDone := w.listenStop, w.listenDone
	beatStop, beatDone := w.beatStop, w.beatDone
	w.mu.Unlock()

	if listenStop == nil {
		return nil
	}

	listenStop()
	<-listenDone

	drained := make(chan struct{})
	go func() {
		w.running.Wait()
		close(drained)
	}()

	var err error
	select {
	case <-drained:
	case <-drainCtx.Done():
		err = w.giveUp(drained)
	}

	// Heartbeats keep the session alive until the handlers have returned.
	beatStop()
	<-beatDone
	w.conn.stop()

	return err
}

// giveUp ends a drain that has reached its deadline. It cancels every attempt that has
// not reported and gives the handlers handlerWait to return.
func (w *Worker) giveUp(drained <-chan struct{}) error {
	w.mu.Lock()
	var given []string
	for _, a := range w.attempts {
		if a.claim() {
			a.cancel()
			given = append(given, a.runID)
		}
	}
	w.mu.Unlock()

	timer := time.NewTimer(handlerWait)
	defer timer.Stop()

	select {
	case <-drained:
	case <-timer.C:
	}

	if len(given) == 0 {
		return nil
	}

	return fmt.Errorf("shutdown: gave up %d runs before they finished: %v", len(given), given)
}

// authenticate puts the bearer token on every call: the API key on Register, and the
// session token that Register returned on the other calls.
func (w *Worker) authenticate(next connect.ClientFunc) connect.ClientFunc {
	return func(ctx context.Context, spec connect.Spec) (connect.ClientStream, error) {
		token := w.client.apiKey
		if spec.Procedure != runcoilv1connect.WorkerServiceRegisterProcedure {
			token = w.sessionToken()
		}

		ctx, info := connect.NewClientContext(ctx)
		info.RequestHeader().Set("Authorization", "Bearer "+token)

		return next(ctx, spec)
	}
}

func (w *Worker) sessionToken() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.session
}

// registerRequest snapshots the Tasks into the message that Register sends.
func (w *Worker) registerRequest() *runcoilv1.RegisterRequest {
	w.mu.Lock()
	defer w.mu.Unlock()

	defs := make([]*runcoilv1.TaskDefinition, 0, len(w.tasks))
	for _, t := range w.tasks {
		defs = append(defs, &runcoilv1.TaskDefinition{Name: t.name})
	}

	return &runcoilv1.RegisterRequest{App: w.app, Tasks: defs, Slots: w.slotCount}
}

// register opens a worker session and keeps its token and heartbeat interval.
func (w *Worker) register(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, registerTimeout)
	defer cancel()

	resp, err := w.rpc.Register(ctx, w.registerRequest())
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}

	hb := resp.GetHeartbeatInterval().AsDuration()
	if hb <= 0 || resp.GetSessionToken() == "" {
		return errors.New("register: server sent no session token or heartbeat interval")
	}

	w.mu.Lock()
	w.session = resp.GetSessionToken()
	w.hbInterval = hb
	w.mu.Unlock()

	w.logger.InfoContext(ctx, "worker registered",
		slog.String("app", w.app),
		slog.String("worker_id", resp.GetWorkerId()),
	)

	return nil
}

// listenLoop keeps a Listen stream open until ctx ends. Each time the stream ends it
// waits, then opens a new one. The wait is the reconnect backoff: it grows while
// attempts fail, and it is reset as soon as the server answers anything, which also
// ends a wait early. An UNAUTHENTICATED answer means the session ended, so the worker
// registers again and opens Listen at once.
func (w *Worker) listenLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	delay := reconnectStart
	immediate := false // the last Listen found the session ended and the worker registered again
	for {
		w.conn.drainAnswered()
		received, err := w.listenOnce(ctx)
		if ctx.Err() != nil {
			return
		}

		if received {
			delay = reconnectStart
		}

		wasImmediate := immediate
		immediate = false
		switch {
		case err == nil:
			w.logger.DebugContext(ctx, "listen stream ended by the server")
		case errorCode(err) == runcoilv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED:
			w.logger.DebugContext(ctx, "listen stream refused: session ended", slog.Any("error", err))

			if rerr := w.register(ctx); rerr != nil {
				w.conn.failed(ctx, rerr, delay)
			} else {
				delay = reconnectStart
				// Open Listen now, but only once in a row: a refusal straight after a
				// registration is a failure, and it waits as any failure does.
				immediate = !wasImmediate
			}
		default:
			w.conn.failed(ctx, err, delay)
		}

		if immediate {
			continue
		}

		running, answered := w.pause(ctx, delay)
		if !running {
			return
		}

		if answered {
			delay = reconnectStart
		} else {
			delay = min(2*delay, reconnectCap)
		}
	}
}

// pause waits for d before the next attempt. It returns early when the server answers.
// running is false when ctx ends first.
func (w *Worker) pause(ctx context.Context, d time.Duration) (running, answered bool) {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false, false
	case <-timer.C:
		return true, false
	case <-w.conn.answered:
		return true, true
	}
}

// listenOnce opens one Listen stream, reporting the attempts the worker runs, and reads
// it until it ends. It reports whether any message arrived. The stream is dropped when
// no message, not even a keepalive, arrives for silentBeats heartbeat intervals.
func (w *Worker) listenOnce(ctx context.Context) (bool, error) {
	silence := silentBeats * w.heartbeatInterval()

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var stalled atomic.Bool
	watchdog := time.AfterFunc(silence, func() {
		stalled.Store(true)
		cancel()
	})
	defer watchdog.Stop()

	stream, err := w.rpc.Listen(streamCtx, &runcoilv1.ListenRequest{Running: w.runningLeases()})
	if err != nil {
		return false, fmt.Errorf("open listen: %w", err)
	}
	defer func() { _ = stream.Close() }()

	received := false
	var streamErr error
	for {
		msg, err := stream.Receive()
		if err != nil {
			streamErr = err
			break
		}

		received = true
		watchdog.Reset(silence)
		w.conn.ok(ctx)
		w.handle(ctx, msg)
	}

	if errors.Is(streamErr, io.EOF) {
		streamErr = nil
	}

	if stalled.Load() {
		return received, fmt.Errorf("listen stream silent for %s", silence)
	}

	if streamErr != nil {
		return received, fmt.Errorf("receive: %w", streamErr)
	}

	return received, nil
}

// handle acts on one message from the Listen stream. A keepalive carries nothing; its
// arrival already reset the watchdog. A kind this SDK does not know is ignored.
func (w *Worker) handle(ctx context.Context, msg *runcoilv1.ListenResponse) {
	if a := msg.GetAssign(); a != nil {
		w.accept(ctx, a)
		return
	}

	if c := msg.GetCancel(); c != nil {
		w.abandon(ctx, c.GetLease())
	}
}

// accept takes an assigned attempt and starts its goroutine. An Assign that repeats a
// held attempt is ignored; one with a newer epoch replaces the held attempt, which is no
// longer the Run's.
func (w *Worker) accept(ctx context.Context, assign *runcoilv1.Assign) {
	lease := assign.GetLease()

	w.mu.Lock()
	defer w.mu.Unlock()

	if held, ok := w.attempts[lease.GetRunId()]; ok {
		if held.epoch == lease.GetEpoch() {
			w.logger.WarnContext(ctx, "duplicate assign ignored", slog.String("run_id", lease.GetRunId()))
			return
		}

		held.abandon()
	}

	attemptCtx, cancel := context.WithCancel(context.Background())
	a := &attempt{
		runID:  lease.GetRunId(),
		epoch:  lease.GetEpoch(),
		cancel: cancel,
		done:   make(chan struct{}),
	}

	st := runstate.NewState(records(assign.GetSteps()))
	st.RunID = a.runID
	st.App = w.app
	st.TaskName = assign.GetTaskName()
	st.Logger = w.logger
	st.Open = a.open
	st.Report = func(_ context.Context, r runstate.StepReport) (runstate.Record, bool) {
		return w.recordStep(a, r)
	}

	a.ctx = runstate.With(attemptCtx, st)
	w.attempts[a.runID] = a

	// A nil task means the worker has no Task with that name; execute reports it.
	t := w.byName[assign.GetTaskName()]

	// Adding here is safe: Shutdown waits for the listen loop to end before it waits on running.
	w.running.Add(1)
	go w.execute(a, t, assign)
}

// runningLeases returns the leases of the attempts the worker still runs, for Listen to
// report.
func (w *Worker) runningLeases() []*runcoilv1.Lease {
	w.mu.Lock()
	defer w.mu.Unlock()

	leases := make([]*runcoilv1.Lease, 0, len(w.attempts))
	for _, a := range w.attempts {
		if a.open() {
			leases = append(leases, a.lease())
		}
	}

	return leases
}

// abandon stops a held attempt that the server has taken back.
func (w *Worker) abandon(ctx context.Context, lease *runcoilv1.Lease) {
	w.mu.Lock()
	a := w.attempts[lease.GetRunId()]
	w.mu.Unlock()

	if a == nil || a.epoch != lease.GetEpoch() {
		return
	}

	w.logger.InfoContext(ctx, "attempt taken back by the server", slog.String("run_id", a.runID))
	a.abandon()
}

// heartbeatLoop sends a heartbeat at the interval the server gave in Register, until ctx
// ends.
func (w *Worker) heartbeatLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	for {
		timer := time.NewTimer(w.heartbeatInterval())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		w.heartbeat(ctx)
	}
}

// heartbeat tells the server the worker process is alive. A session that ended is left to
// the listen loop, which registers again.
func (w *Worker) heartbeat(ctx context.Context) {
	callCtx, cancel := context.WithTimeout(ctx, w.callTimeout())
	defer cancel()

	if _, err := w.rpc.Heartbeat(callCtx, &runcoilv1.HeartbeatRequest{}); err != nil {
		if ctx.Err() == nil {
			w.conn.failed(ctx, err, w.heartbeatInterval())
		}

		return
	}

	w.conn.ok(ctx)
}

// heartbeatInterval returns the interval of the last registration, or the registration
// timeout before one.
func (w *Worker) heartbeatInterval() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.hbInterval <= 0 {
		return registerTimeout
	}

	return w.hbInterval
}

// callTimeout bounds one unary call: about one heartbeat interval.
func (w *Worker) callTimeout() time.Duration {
	return max(w.heartbeatInterval(), minCallTimeout)
}

// reportBudget bounds the tries of one report: as long as the server keeps a silent
// worker's session.
func (w *Worker) reportBudget() time.Duration {
	return silentBeats * w.heartbeatInterval()
}

// retryCall runs op, and runs it again on a retryable failure: a transport failure, a
// server outage, or an internal server error. It stops once the next wait would pass
// budget, and as soon as parent ends. Each try has its own deadline. A failure that
// carries a Runcoil error code, such as STALE_LEASE, is never retried.
func (w *Worker) retryCall(parent context.Context, budget time.Duration, op func(ctx context.Context) error) error {
	deadline := time.Now().Add(budget)
	backoff := retryStart
	for try := 1; ; try++ {
		ctx, cancel := context.WithTimeout(parent, w.callTimeout())
		err := op(ctx)
		cancel()

		if err == nil || !retryable(err) || parent.Err() != nil {
			return err
		}

		if time.Now().Add(backoff).After(deadline) {
			return err
		}

		w.logger.DebugContext(parent, "call failed; retrying",
			slog.Int("try", try),
			slog.Duration("next_delay", backoff),
			slog.Any("error", err),
		)

		if !sleep(parent, backoff) {
			return err
		}

		backoff = min(2*backoff, retryCap)
	}
}

// retryable reports whether a failed call may succeed when it is sent again.
func retryable(err error) bool {
	if errorCode(err) != runcoilv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return false
	}

	switch connect.CodeOf(err) {
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded, connect.CodeUnknown, connect.CodeInternal:
		return true
	default:
		return false
	}
}

// errorCode returns the Runcoil error code in the details of err, or
// ERROR_CODE_UNSPECIFIED when there is none.
func errorCode(err error) runcoilv1.ErrorCode {
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return runcoilv1.ErrorCode_ERROR_CODE_UNSPECIFIED
	}

	for _, d := range ce.Details() {
		msg, err := connectproto.UnmarshalErrorDetail(d)
		if err != nil {
			continue
		}

		if detail, ok := msg.(*runcoilv1.ErrorDetail); ok {
			return detail.GetCode()
		}
	}

	return runcoilv1.ErrorCode_ERROR_CODE_UNSPECIFIED
}

// sleep waits for d and reports false when ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// connection follows the state of the worker's connection to the server, not each
// attempt to reach it. The first failure of an outage is one warning; the failed
// attempts after it are debug lines; a reminder repeats the warning on a fixed interval;
// the first success after the outage is one info line. It is safe for concurrent use by
// the listen loop, the heartbeat loop, and the attempt goroutines.
type connection struct {
	logger *slog.Logger
	app    string

	mu       sync.Mutex
	down     bool
	since    time.Time
	attempts int
	lastErr  error
	gen      uint64 // identifies the current outage; a timer of an older one does nothing

	// answered signals that the server has answered: the listen loop's wait ends early.
	answered chan struct{}
}

// failed records a failed attempt to reach the server. next is the wait before the next
// attempt. The first failure of an outage is logged as a warning.
func (c *connection) failed(ctx context.Context, err error, next time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastErr = err
	if !c.down {
		c.down = true
		c.since = time.Now()
		c.attempts = 1
		c.gen++
		c.logger.WarnContext(ctx, "server connection lost",
			slog.Any("error", err),
			slog.String("app", c.app),
		)

		c.armReminder()

		return
	}

	c.attempts++
	c.logger.DebugContext(ctx, "server connection attempt failed",
		slog.Int("attempt", c.attempts),
		slog.Duration("next_delay", next),
		slog.Any("error", err),
	)
}

// ok records that the server answered: the listen stream delivered a message, or a
// heartbeat succeeded. It ends an outage with one info line.
func (c *connection) ok(ctx context.Context) {
	select {
	case c.answered <- struct{}{}:
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.down {
		return
	}

	c.logger.InfoContext(ctx, "server connection restored",
		slog.Duration("outage", time.Since(c.since)),
		slog.Int("attempts", c.attempts),
	)

	c.down = false
	c.attempts = 0
	c.lastErr = nil
	c.gen++
}

// drainAnswered discards an answer that the listen loop has not used yet, so that only
// answers from now on end its next wait.
func (c *connection) drainAnswered() {
	select {
	case <-c.answered:
	default:
	}
}

// stop ends the reminders when the worker shuts down.
func (c *connection) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.down = false
	c.gen++
}

// armReminder schedules the next reminder of the current outage. c.mu is held.
func (c *connection) armReminder() {
	gen := c.gen
	time.AfterFunc(reminderEvery, func() { c.remind(gen) })
}

// remind logs that the outage still lasts, and schedules the next reminder.
func (c *connection) remind(gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.down || gen != c.gen {
		return
	}

	c.logger.Warn("server still unreachable",
		slog.Duration("down_for", time.Since(c.since)),
		slog.Int("attempts", c.attempts),
		slog.Any("error", c.lastErr),
	)

	c.armReminder()
}
