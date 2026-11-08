package step

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

// fakeRun is a run state whose reports are stored as the server would store them.
type fakeRun struct {
	st      *runstate.State
	reports []runstate.StepReport
	open    bool
	refuse  bool
}

func newFakeRun(recorded map[string]runstate.Record) (*fakeRun, context.Context) {
	f := &fakeRun{st: runstate.NewState(recorded), open: true}
	f.st.RunID = "run-1"
	f.st.Logger = slog.New(slog.DiscardHandler)
	f.st.Open = func() bool { return f.open }
	f.st.Report = func(_ context.Context, r runstate.StepReport) (runstate.Record, bool) {
		f.reports = append(f.reports, r)
		if f.refuse {
			f.open = false
			return runstate.Record{}, false
		}

		if r.Failure != nil {
			return runstate.Record{Kind: r.Failure.Kind, Message: r.Failure.Message}, true
		}

		return runstate.Record{Succeeded: true, Output: r.Output}, true
	}

	return f, runstate.With(context.Background(), f.st)
}

// unwound reports whether fn ended the attempt by unwinding.
func unwound(fn func()) (ok bool) {
	defer func() {
		ok = runstate.IsUnwind(recover())
	}()

	fn()

	return false
}

func TestDoOutsideARunCallsFn(t *testing.T) {
	want := errors.New("plain")

	got, err := Do(context.Background(), "charge", func(context.Context) (int, error) { return 7, want })

	assert.Equal(t, 7, got)
	assert.Same(t, want, err)
}

func TestDoReportsAndReturnsTheStoredValue(t *testing.T) {
	f, ctx := newFakeRun(nil)

	got, err := Do(ctx, "charge", func(context.Context) (map[string]int, error) { return map[string]int{"amount": 3}, nil })
	require.NoError(t, err)

	assert.Equal(t, map[string]int{"amount": 3}, got)
	require.Len(t, f.reports, 1)
	assert.Equal(t, "charge", f.reports[0].StepID)
	assert.JSONEq(t, `{"amount":3}`, string(f.reports[0].Output))
	assert.False(t, f.reports[0].StartedAt.IsZero())
}

func TestDoReplaysARecord(t *testing.T) {
	f, ctx := newFakeRun(map[string]runstate.Record{
		"charge":   {Succeeded: true, Output: []byte(`"stored"`)},
		"charge#2": {Kind: runstate.KindError, Message: "declined"},
	})

	got, err := Do(ctx, "charge", func(context.Context) (string, error) { return "new", nil })
	require.NoError(t, err)
	assert.Equal(t, "stored", got)

	_, err = Do(ctx, "charge", func(context.Context) (string, error) { return "new", nil })

	var stepErr *Error
	require.ErrorAs(t, err, &stepErr)
	assert.Equal(t, &Error{StepID: "charge#2", Kind: runstate.KindError, Message: "declined"}, stepErr)
	assert.Empty(t, f.reports, "a stored Step runs no function and sends no report")
}

func TestDoReportsAFailure(t *testing.T) {
	tests := []struct {
		name    string
		fn      func(context.Context) (func(), error)
		kind    string
		message string
	}{
		{
			name:    "an error",
			fn:      func(context.Context) (func(), error) { return nil, errors.New("declined") },
			kind:    runstate.KindError,
			message: "declined",
		},
		{
			name:    "a panic",
			fn:      func(context.Context) (func(), error) { panic("boom") },
			kind:    runstate.KindPanic,
			message: "step panicked: boom",
		},
		{
			name:    "an output that is not JSON",
			fn:      func(context.Context) (func(), error) { return func() {}, nil },
			kind:    runstate.KindOutputNotEncodable,
			message: "encode output: json: unsupported type: func()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ctx := newFakeRun(nil)

			_, err := Do(ctx, "charge", tt.fn)

			var stepErr *Error
			require.ErrorAs(t, err, &stepErr)
			assert.Equal(t, tt.kind, stepErr.Kind)
			assert.Equal(t, tt.message, stepErr.Message)

			require.Len(t, f.reports, 1)
			assert.Equal(t, &runstate.Failure{Kind: tt.kind, Message: tt.message}, f.reports[0].Failure)
		})
	}
}

func TestDoUnwindsWhenTheAttemptEnded(t *testing.T) {
	t.Run("before the Step", func(t *testing.T) {
		f, ctx := newFakeRun(nil)
		f.open = false

		called := false
		assert.True(t, unwound(func() {
			_, _ = Do(ctx, "charge", func(context.Context) (int, error) { called = true; return 1, nil })
		}))
		assert.False(t, called)
	})

	t.Run("while the Step ran", func(t *testing.T) {
		f, ctx := newFakeRun(nil)

		assert.True(t, unwound(func() {
			_, _ = Do(ctx, "charge", func(context.Context) (int, error) { f.open = false; return 1, nil })
		}))
		assert.Empty(t, f.reports)
	})

	t.Run("when the report is refused", func(t *testing.T) {
		f, ctx := newFakeRun(nil)
		f.refuse = true

		assert.True(t, unwound(func() {
			_, _ = Do(ctx, "charge", func(context.Context) (int, error) { return 1, nil })
		}))
	})
}

func TestDoNeedsAnID(t *testing.T) {
	_, ctx := newFakeRun(nil)

	_, err := Do(ctx, "", func(context.Context) (int, error) { return 1, nil })

	assert.EqualError(t, err, "step: id is required")
}
