package runcoil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

const (
	reconnectStart = 500 * time.Millisecond
	reconnectCap   = 10 * time.Second
	// silentBeats is how many heartbeat intervals a Listen stream may stay silent before
	// the worker drops it. The server sends a keepalive every interval.
	silentBeats = 3
)

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
		case errors.Is(err, errSessionEnded) || errorCode(err) == runcoilv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED:
			w.logger.DebugContext(ctx, "session ended; registering again", slog.Any("error", err))

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

	w.mu.Lock()
	w.streamCancel = cancel
	w.mu.Unlock()

	var stalled atomic.Bool
	watchdog := time.AfterFunc(silence, func() {
		stalled.Store(true)
		cancel()
	})
	defer watchdog.Stop()

	running := w.runningLeases()
	stream, err := w.rpc.Listen(streamCtx, &runcoilv1.ListenRequest{Running: running})
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

		if !received {
			// The stream is open only once the server sends something: Listen returns
			// before the connection is made.
			w.logger.DebugContext(ctx, "listen stream attached", slog.Int("running", len(running)))
		}

		received = true
		watchdog.Reset(silence)
		w.conn.ok(ctx)
		w.handle(ctx, msg)
	}

	if errors.Is(streamErr, io.EOF) {
		streamErr = nil
	}

	if w.sessionEnded.Load() {
		return received, errSessionEnded
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

		held.end()
	}

	attemptCtx, cancel := context.WithCancel(context.Background())
	a := &attempt{
		runID: lease.GetRunId(),
		epoch: lease.GetEpoch(),
		logger: w.logger.With(
			slog.String("run_id", lease.GetRunId()),
			slog.String("task_name", assign.GetTaskName()),
			slog.Int64("epoch", lease.GetEpoch()),
		),
		cancel: cancel,
	}

	st := runstate.NewState(stepRecordsFromProto(assign.GetSteps()))
	st.RunID = a.runID
	st.App = w.app
	st.TaskName = assign.GetTaskName()
	st.Logger = a.logger
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

	a.logger.InfoContext(ctx, "attempt taken back by the server")
	a.end()
}
