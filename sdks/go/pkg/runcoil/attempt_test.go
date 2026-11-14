package runcoil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/sdks/go/pkg/retry"
	"github.com/runcoil/runcoil/sdks/go/pkg/step"
)

func TestAScheduledRetryStopsTheHandler(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	f.set(func(f *fakeServer) {
		f.stepAnswer = func(*runcoilv1.CompleteStepRequest) (*runcoilv1.CompleteStepResponse, error) {
			return &runcoilv1.CompleteStepResponse{Result: &runcoilv1.CompleteStepResponse_RetryScheduled{
				RetryScheduled: &runcoilv1.RetryScheduled{NextAttemptAt: timestamppb.Now(), TryCount: 1},
			}}, nil
		}
	})

	var after atomic.Bool
	w := startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			_, err := step.Do(ctx, "charge", func(context.Context) (int, error) {
				return 0, retry.After(errors.New("rate limited"), 30*time.Second)
			}, step.Retry(retry.Policy{MaxAttempts: 3, Delays: []time.Duration{time.Second}}))
			after.Store(true)

			return orderOutput{}, err
		}),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)

	f.noRun(t)
	assert.False(t, after.Load(), "the handler stops inside the Step that will be retried")
	require.Eventually(t, func() bool { return len(w.runningLeases()) == 0 }, time.Second, 10*time.Millisecond,
		"the attempt is over, so Listen no longer reports it")
	assert.Empty(t, w.slots, "the slot is free")

	_, _, _, steps := f.snapshot()
	require.Len(t, steps, 1)
	req := steps[0]
	assert.Equal(t, "rate limited", req.GetError().GetMessage())
	assert.Equal(t, 30*time.Second, req.GetError().GetRetryAfter().AsDuration())
	assert.False(t, req.GetError().GetNonRetriable())
	assert.Equal(t, uint32(3), req.GetRetry().GetMaxAttempts())
	assert.Equal(t, []*durationpb.Duration{durationpb.New(time.Second)}, req.GetRetry().GetDelays())
}

func TestANonRetriableErrorIsMarked(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	declined := errors.New("card declined")
	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			_, err := step.Do(ctx, "charge", func(context.Context) (int, error) { return 0, retry.NonRetriable(declined) })
			return orderOutput{}, err
		}),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)

	run := f.waitRun(t)
	assert.Equal(t, "charge", run.GetError().GetStepId())

	_, _, _, steps := f.snapshot()
	require.Len(t, steps, 1)
	assert.True(t, steps[0].GetError().GetNonRetriable())
	assert.Nil(t, steps[0].GetError().GetRetryAfter())
	assert.Nil(t, steps[0].GetRetry(), "no step.Retry: the Task's policy")
}

// tooLargeErrors are the two ways the server refuses an output for its size: over its
// limit, and over the largest message it reads.
func tooLargeErrors(t *testing.T) map[string]error {
	t.Helper()

	return map[string]error{
		"over the limit": codedError(t, connect.CodeInvalidArgument, runcoilv1.ErrorCode_ERROR_CODE_OUTPUT_TOO_LARGE,
			"output is 2000000 bytes, over the limit of 1048576 bytes"),
		"over the read limit": connect.NewError(connect.CodeResourceExhausted, "message is larger than configured max"),
	}
}

func TestAStepOutputTooLargeFailsTheStep(t *testing.T) {
	for name, tooLarge := range tooLargeErrors(t) {
		t.Run(name, func(t *testing.T) {
			f, url := newFakeServer(t)
			st := newFakeStream()
			f.streams <- st

			f.set(func(f *fakeServer) {
				f.stepAnswer = func(req *runcoilv1.CompleteStepRequest) (*runcoilv1.CompleteStepResponse, error) {
					if req.GetError() == nil {
						return nil, tooLarge
					}

					rec := storedRecord(req)
					rec.GetFailure().Code = runcoilv1.Failure_CODE_OUTPUT_TOO_LARGE

					return &runcoilv1.CompleteStepResponse{Result: &runcoilv1.CompleteStepResponse_Record{Record: rec}}, nil
				}
			})

			startWorker(t, url, map[string]Handler{
				"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
					_, err := step.Do(ctx, "big", func(context.Context) (string, error) { return "a large output", nil })

					var stepErr *step.Error
					require.ErrorAs(t, err, &stepErr)
					assert.Equal(t, "output_too_large", stepErr.Kind)

					return orderOutput{}, err
				}),
			})

			st.send <- assignMsg("run-1", 1, "order", `{}`)

			run := f.waitRun(t)
			assert.Equal(t, "big", run.GetError().GetStepId())
			assert.Contains(t, run.GetError().GetMessage(), "step output of 16 bytes is too large")

			_, _, _, steps := f.snapshot()
			require.Len(t, steps, 2)
			assert.NotNil(t, steps[0].GetOutput())
			assert.Nil(t, steps[1].GetOutput())
			assert.Equal(t, runcoilv1.ErrorKind_ERROR_KIND_OUTPUT_TOO_LARGE, steps[1].GetError().GetKind())
			assert.True(t, steps[1].GetError().GetNonRetriable())
		})
	}
}

func TestARunOutputTooLargeFailsTheRun(t *testing.T) {
	for name, tooLarge := range tooLargeErrors(t) {
		t.Run(name, func(t *testing.T) {
			f, url := newFakeServer(t)
			st := newFakeStream()
			f.streams <- st

			f.set(func(f *fakeServer) {
				f.runRefuse = func(req *runcoilv1.CompleteRunRequest) error {
					if req.GetError() == nil {
						return tooLarge
					}

					return nil
				}
			})

			startWorker(t, url, map[string]Handler{
				"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) {
					return orderOutput{Receipt: "r-1"}, nil
				}),
			})

			st.send <- assignMsg("run-1", 1, "order", `{}`)

			run := f.waitRun(t)
			assert.Equal(t, runcoilv1.ErrorKind_ERROR_KIND_OUTPUT_TOO_LARGE, run.GetError().GetKind())
			assert.Contains(t, run.GetError().GetMessage(), "run output of 17 bytes is too large")
			assert.Equal(t, "run-1", run.GetLease().GetRunId())
			assert.Len(t, f.sentRuns(), 2)
		})
	}
}

func TestARefusedRunResultFailsTheRun(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	invalid := codedError(t, connect.CodeInvalidArgument, runcoilv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, "step_id is not valid")
	f.set(func(f *fakeServer) {
		f.runRefuse = func(req *runcoilv1.CompleteRunRequest) error {
			if req.GetError().GetStepId() != "" {
				return invalid
			}

			return nil
		}
	})

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) {
			return orderOutput{}, &step.Error{StepID: "bad\x00id", Message: "declined"}
		}),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)

	run := f.waitRun(t)
	assert.Equal(t, runcoilv1.ErrorKind_ERROR_KIND_ERROR, run.GetError().GetKind())
	assert.Equal(t, "the server refused the run result: step_id is not valid", run.GetError().GetMessage())
	assert.Empty(t, run.GetError().GetStepId())
}

func TestARunResultRefusedTwiceIsDropped(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	invalid := codedError(t, connect.CodeInvalidArgument, runcoilv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, "no")
	f.set(func(f *fakeServer) {
		f.runRefuse = func(*runcoilv1.CompleteRunRequest) error { return invalid }
	})

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { return orderOutput{}, nil }),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)

	require.Eventually(t, func() bool { return len(f.sentRuns()) == 2 }, 5*time.Second, 10*time.Millisecond)
	f.noRun(t)
	assert.Len(t, f.sentRuns(), 2, "the failure that replaces a refused result is sent once")
}

func TestARefusedStepReportFailsTheRun(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	invalid := codedError(t, connect.CodeInvalidArgument, runcoilv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, "step id must not contain a NUL byte")
	f.set(func(f *fakeServer) {
		f.stepAnswer = func(*runcoilv1.CompleteStepRequest) (*runcoilv1.CompleteStepResponse, error) { return nil, invalid }
	})

	var after atomic.Bool
	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			_, err := step.Do(ctx, "charge", func(context.Context) (int, error) { return 1, nil })
			after.Store(true)

			return orderOutput{}, err
		}),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)

	run := f.waitRun(t)
	assert.Equal(t, runcoilv1.ErrorKind_ERROR_KIND_ERROR, run.GetError().GetKind())
	assert.Equal(t, `the server refused the report of step "charge": step id must not contain a NUL byte`, run.GetError().GetMessage())
	f.noRun(t)
	assert.False(t, after.Load(), "the handler does not run past a refused Step")

	_, _, _, steps := f.snapshot()
	assert.Len(t, steps, 1, "a refused Step report is not sent again")
}
