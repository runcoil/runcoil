package runcoil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

// errAttemptEnded is what callHandler returns when the attempt ended while the handler
// ran. Nothing is reported for such an attempt.
var errAttemptEnded = errors.New("attempt ended")

// attempt is one Run attempt that this worker holds, from its Assign until its result is
// reported or it ends.
type attempt struct {
	runID string
	epoch int64

	// ctx is the handler's context. It is cancelled when the attempt ends.
	ctx    context.Context
	cancel context.CancelFunc

	mu sync.Mutex
	// claimed is set by the one caller that sends the result or ends the attempt.
	claimed bool
	// ended is set once the attempt is no longer this worker's.
	ended bool
}

// claim returns true for the first caller only. That caller sends the attempt's result;
// every later caller must send nothing.
func (a *attempt) claim() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.claimed {
		return false
	}

	a.claimed = true

	return true
}

// open reports whether the attempt is still this worker's. An attempt whose result is
// being sent is open: Listen still reports it, so the server keeps its lease.
func (a *attempt) open() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	return !a.ended
}

// end makes the attempt no longer this worker's. Its result would be refused, so none is
// sent, and its handler's context is cancelled. It reports whether this call ended an
// attempt whose result was not claimed yet.
func (a *attempt) end() bool {
	a.mu.Lock()
	unclaimed := !a.claimed
	a.claimed = true
	a.ended = true
	a.mu.Unlock()

	a.cancel()

	return unclaimed
}

func (a *attempt) lease() *runcoilv1.Lease {
	return &runcoilv1.Lease{RunId: a.runID, Epoch: a.epoch}
}

// panicError is a panic that the worker recovered from a handler.
type panicError struct {
	value any
	stack []byte
}

func (e *panicError) Error() string { return fmt.Sprintf("handler panicked: %v", e.value) }

// execute runs the handler of an attempt and reports its result. It runs in its own
// goroutine, one per attempt.
func (w *Worker) execute(a *attempt, t *task, assign *runcoilv1.Assign) {
	defer w.running.Done()
	defer w.forget(a)
	defer a.cancel()

	logger := w.logger.With(slog.String("run_id", a.runID))

	if t == nil {
		w.report(a, failedRun(runcoilv1.ErrorKind_ERROR_KIND_ERROR,
			fmt.Sprintf("task %q is not registered by this worker", assign.GetTaskName()), ""))

		return
	}

	if !w.takeSlot(a.ctx) {
		return
	}
	defer func() { <-w.slots }()

	// The slot and the end of the attempt may be ready together, and select picks either.
	if !a.open() {
		return
	}

	out, err := callHandler(a.ctx, t.handler, assign.GetInput())
	if errors.Is(err, errAttemptEnded) {
		logger.DebugContext(a.ctx, "attempt ended while its handler ran; no report")
		return
	}

	var pe *panicError
	if errors.As(err, &pe) {
		logger.ErrorContext(a.ctx, "handler panicked",
			slog.String("task_name", t.name),
			slog.Any("panic", pe.value),
			slog.String("stack", string(pe.stack)),
		)
	}

	w.report(a, resultOf(out, err))
}

// takeSlot waits for one of the worker's handler slots. It returns false when ctx ends
// first.
func (w *Worker) takeSlot(ctx context.Context) bool {
	select {
	case w.slots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

// report sends the result of an attempt unless the attempt has ended.
func (w *Worker) report(a *attempt, req *runcoilv1.CompleteRunRequest) {
	if !a.claim() {
		w.logger.InfoContext(a.ctx, "attempt ended before its result was reported", slog.String("run_id", a.runID))
		return
	}

	req.Lease = a.lease()

	// The attempt is claimed, so its context no longer ends it: the report runs until it
	// is sent, or until Shutdown cuts it short.
	ctx := w.reportCtx
	session := w.sessionToken()
	err := w.retryCall(ctx, w.reportBudget(), func(ctx context.Context) error {
		if _, err := w.rpc.CompleteRun(ctx, req); err != nil {
			return fmt.Errorf("complete run: %w", err)
		}

		return nil
	})
	if err == nil {
		return
	}

	logger := w.logger.With(slog.String("run_id", a.runID))
	switch {
	case errorCode(err) == runcoilv1.ErrorCode_ERROR_CODE_STALE_LEASE:
		logger.InfoContext(ctx, "run result refused: lease is stale")
	case w.endSession(ctx, session, err):
		logger.WarnContext(ctx, "run result refused: worker session ended")
	default:
		logger.ErrorContext(ctx, "report run result failed", slog.Any("error", err))
	}
}

// forget drops the attempt from the held set, so Listen stops reporting it.
func (w *Worker) forget(a *attempt) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.attempts[a.runID] == a {
		delete(w.attempts, a.runID)
	}
}

// callHandler runs the handler and turns a panic into an error. The panic that ends an
// attempt becomes errAttemptEnded.
func callHandler(ctx context.Context, h Handler, input []byte) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			if runstate.IsUnwind(r) {
				out, err = nil, errAttemptEnded
				return
			}

			out, err = nil, &panicError{value: r, stack: debug.Stack()}
		}
	}()

	return h.call(ctx, input)
}

// resultOf turns a handler's output or error into the result CompleteRun sends.
func resultOf(out []byte, err error) *runcoilv1.CompleteRunRequest {
	if err == nil {
		return &runcoilv1.CompleteRunRequest{Result: &runcoilv1.CompleteRunRequest_Output{Output: out}}
	}

	var se *runstate.StepError
	if errors.As(err, &se) {
		// The handler returned a Step's final error.
		return failedRun(runcoilv1.ErrorKind_ERROR_KIND_ERROR, err.Error(), se.StepID)
	}

	var pe *panicError
	if errors.As(err, &pe) {
		return failedRun(runcoilv1.ErrorKind_ERROR_KIND_PANIC, pe.Error(), "")
	}

	var ee *encodeError
	if errors.As(err, &ee) {
		return failedRun(runcoilv1.ErrorKind_ERROR_KIND_OUTPUT_NOT_ENCODABLE, ee.Error(), "")
	}

	return failedRun(runcoilv1.ErrorKind_ERROR_KIND_ERROR, err.Error(), "")
}

// failedRun builds a result that fails the Run. stepID is set when the handler returned
// the final error of that Step.
func failedRun(kind runcoilv1.ErrorKind, message, stepID string) *runcoilv1.CompleteRunRequest {
	return &runcoilv1.CompleteRunRequest{Result: &runcoilv1.CompleteRunRequest_Error{
		Error: &runcoilv1.RunError{Kind: kind, Message: message, StepId: stepID},
	}}
}

// recordStep sends the outcome of one Step of the attempt and returns the stored record.
// It returns false when the attempt ended, or when the report could not be made: the
// attempt is then ended here, and the handler must stop.
func (w *Worker) recordStep(a *attempt, r runstate.StepReport) (runstate.Record, bool) {
	logger := w.logger.With(
		slog.String("run_id", a.runID),
		slog.String("step_id", r.StepID),
	)

	req := &runcoilv1.CompleteStepRequest{
		Lease:     a.lease(),
		StepId:    r.StepID,
		StartedAt: timestamppb.New(r.StartedAt),
		EndedAt:   timestamppb.New(r.EndedAt),
	}

	if r.Failure != nil {
		req.Outcome = &runcoilv1.CompleteStepRequest_Error{Error: &runcoilv1.StepError{
			Kind:    errorKind(r.Failure.Kind),
			Message: r.Failure.Message,
		}}
	} else {
		req.Outcome = &runcoilv1.CompleteStepRequest_Output{Output: r.Output}
	}

	var resp *runcoilv1.CompleteStepResponse
	session := w.sessionToken()
	err := w.retryCall(a.ctx, w.reportBudget(), func(ctx context.Context) error {
		var err error
		if resp, err = w.rpc.CompleteStep(ctx, req); err != nil {
			return fmt.Errorf("complete step: %w", err)
		}

		return nil
	})
	if err != nil {
		switch {
		case errorCode(err) == runcoilv1.ErrorCode_ERROR_CODE_STALE_LEASE:
			logger.InfoContext(a.ctx, "step report refused: lease is stale")
		case w.endSession(a.ctx, session, err):
			logger.WarnContext(a.ctx, "step report refused: worker session ended; attempt ended")
		case a.ctx.Err() == nil:
			logger.ErrorContext(a.ctx, "step report failed; attempt ended", slog.Any("error", err))
		}

		a.end()

		return runstate.Record{}, false
	}

	rec := resp.GetRecord()
	if rec == nil {
		logger.ErrorContext(a.ctx, "step report answered without a record; attempt ended")
		a.end()

		return runstate.Record{}, false
	}

	return recordOf(rec), true
}

// records converts the journal an Assign carries, by Step ID.
func records(steps []*runcoilv1.StepRecord) map[string]runstate.Record {
	recorded := make(map[string]runstate.Record, len(steps))
	for _, rec := range steps {
		recorded[rec.GetStepId()] = recordOf(rec)
	}

	return recorded
}

// recordOf converts a stored Step record from the server.
func recordOf(rec *runcoilv1.StepRecord) runstate.Record {
	failure := rec.GetFailure()
	if failure == nil {
		return runstate.Record{Succeeded: true, Output: rec.GetOutput()}
	}

	return runstate.Record{Kind: failureKind(failure.GetCode()), Message: failure.GetMessage()}
}

func errorKind(kind string) runcoilv1.ErrorKind {
	switch kind {
	case runstate.KindPanic:
		return runcoilv1.ErrorKind_ERROR_KIND_PANIC
	case runstate.KindOutputNotEncodable:
		return runcoilv1.ErrorKind_ERROR_KIND_OUTPUT_NOT_ENCODABLE
	default:
		return runcoilv1.ErrorKind_ERROR_KIND_ERROR
	}
}

func failureKind(code runcoilv1.Failure_Code) string {
	switch code {
	case runcoilv1.Failure_CODE_PANIC:
		return runstate.KindPanic
	case runcoilv1.Failure_CODE_OUTPUT_NOT_ENCODABLE:
		return runstate.KindOutputNotEncodable
	default:
		return runstate.KindError
	}
}
