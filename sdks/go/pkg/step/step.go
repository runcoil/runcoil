package step

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

// Error is the final error of a Step that failed. Do returns it for such a Step, also on
// replay, and errors.As finds it. It is rebuilt from the stored text, so errors.Is does
// not match the application's own error values.
type Error = runstate.StepError

// Do runs fn and returns its result. Wrap each call with a side effect, such as a payment
// or an email, in Do. Its outcome is stored, so a Run that runs again returns the stored
// value, or the stored *Error, and does not call fn again. A Step that fails returns
// *Error, and the handler may run a fallback or return it to fail the Run.
//
// The Step is stored under id the first time; a later Step with the same id in the same
// Run gets a counter (charge, charge#2).
//
// Outside a Run, Do runs fn and records nothing, and returns its result and error
// untouched.
//
// A handler that calls Do must follow two rules:
//
//   - Call Do on the handler's own goroutine. The SDK ends an attempt by panicking inside
//     Do, and only the handler's goroutine recovers that panic.
//   - Code that recovers panics must re-panic any value it does not recognise. A recover
//     that swallows the SDK's value would let the handler run on after its attempt ended.
//
// Once the server takes the attempt back, Do stops the handler and reports nothing. A
// Step function must return promptly when its context is cancelled.
func Do[T any](ctx context.Context, id string, fn func(ctx context.Context) (T, error)) (T, error) {
	st, inRun := runstate.From(ctx)
	if !inRun {
		return fn(ctx)
	}

	var zero T
	if id == "" {
		return zero, errors.New("step: id is required")
	}

	if !st.Open() {
		runstate.Unwind()
	}

	stepID := st.StepID(id)
	if stepID != id {
		st.Logger.WarnContext(ctx, "step id used again in this run; stored with a counter",
			slog.String("step_id", stepID),
		)
	}

	if rec, ok := st.Lookup(stepID); ok {
		st.Logger.DebugContext(ctx, "step replayed",
			slog.String("step_id", stepID),
			slog.String("status", recordStatus(rec)),
		)

		return result[T](stepID, rec)
	}

	startedAt := time.Now()
	out := call(ctx, fn)
	endedAt := time.Now()

	if !st.Open() {
		// The attempt ended while fn ran. Whatever fn returned is not reported.
		runstate.Unwind()
	}

	if out.panicked {
		st.Logger.ErrorContext(ctx, "step panicked",
			slog.String("step_id", stepID),
			slog.Any("panic", out.panicValue),
			slog.String("stack", string(out.stack)),
		)
	}

	rep := report(stepID, out)
	rep.StartedAt, rep.EndedAt = startedAt, endedAt

	rec, ok := st.Report(ctx, rep)
	if !ok || !st.Open() {
		// The attempt ended while the Step was reported: the code after it must not run.
		runstate.Unwind()
	}

	st.Store(stepID, rec)

	st.Logger.DebugContext(ctx, "step completed",
		slog.String("step_id", stepID),
		slog.String("status", recordStatus(rec)),
		slog.Duration("duration", endedAt.Sub(startedAt)),
	)

	return result[T](stepID, rec)
}

// recordStatus names the outcome of a stored record for the logs.
func recordStatus(rec runstate.Record) string {
	if rec.Succeeded {
		return "succeeded"
	}

	return "failed"
}

// outcome is what fn returned, or the panic it raised.
type outcome[T any] struct {
	value      T
	err        error
	panicked   bool
	panicValue any
	stack      []byte
}

// call runs fn and recovers its panic. The panic that ends an attempt is not fn's
// failure, so it is raised again.
func call[T any](ctx context.Context, fn func(ctx context.Context) (T, error)) (out outcome[T]) {
	defer func() {
		if r := recover(); r != nil {
			if runstate.IsUnwind(r) {
				panic(r)
			}

			out = outcome[T]{panicked: true, panicValue: r, stack: debug.Stack()}
		}
	}()

	out.value, out.err = fn(ctx)

	return out
}

// report builds the report of a Step from what its function returned or the panic it
// raised.
func report[T any](stepID string, out outcome[T]) runstate.StepReport {
	rep := runstate.StepReport{StepID: stepID}

	switch {
	case out.panicked:
		rep.Failure = &runstate.Failure{Kind: runstate.KindPanic, Message: fmt.Sprintf("step panicked: %v", out.panicValue)}
	case out.err != nil:
		rep.Failure = &runstate.Failure{Kind: runstate.KindError, Message: out.err.Error()}
	default:
		b, err := json.Marshal(out.value)
		if err != nil {
			rep.Failure = &runstate.Failure{Kind: runstate.KindOutputNotEncodable, Message: fmt.Sprintf("encode output: %v", err)}
		} else {
			rep.Output = b
		}
	}

	return rep
}

// result turns a stored record into what Do returns: the decoded value, or the *Error of
// a failed Step.
func result[T any](stepID string, rec runstate.Record) (T, error) {
	var zero T
	if !rec.Succeeded {
		return zero, &Error{StepID: stepID, Kind: rec.Kind, Message: rec.Message}
	}

	var v T
	if err := json.Unmarshal(rec.Output, &v); err != nil {
		return zero, fmt.Errorf("step %q: decode stored output: %w", stepID, err)
	}

	return v, nil
}
