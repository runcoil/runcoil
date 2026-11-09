package runcoil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/sdks/go/pkg/step"
)

func TestRegisterCarriesTheTaskOptions(t *testing.T) {
	f, url := newFakeServer(t)
	f.streams <- newFakeStream()

	client, err := NewClient(URL(url), APIKey(testAPIKey))
	require.NoError(t, err)

	w, err := NewWorker(client, "shop")
	require.NoError(t, err)

	h := HandlerFunc(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })
	require.NoError(t, w.NewTask("charge", h,
		TaskRetry(step.RetryPolicy{MaxAttempts: 5, Delays: []time.Duration{time.Second, time.Minute}}),
		StartDeadline(10*time.Minute),
	))
	require.NoError(t, w.NewTask("plain", h))

	require.NoError(t, w.Start(context.Background()))
	t.Cleanup(func() { _ = w.Shutdown(context.Background()) })

	f.mu.Lock()
	defs := f.regReqs[0].GetTasks()
	f.mu.Unlock()

	require.Len(t, defs, 2)
	assert.Equal(t, uint32(5), defs[0].GetRetry().GetMaxAttempts())
	assert.Equal(t, []time.Duration{time.Second, time.Minute},
		[]time.Duration{defs[0].GetRetry().GetDelays()[0].AsDuration(), defs[0].GetRetry().GetDelays()[1].AsDuration()})
	assert.Equal(t, 10*time.Minute, defs[0].GetStartDeadline().AsDuration())
	assert.Nil(t, defs[1].GetRetry(), "no policy: the server's default")
	assert.Nil(t, defs[1].GetStartDeadline(), "no deadline")
}

func TestNewTaskValidatesItsOptions(t *testing.T) {
	client, err := NewClient(URL("http://localhost:7070"), APIKey("k"))
	require.NoError(t, err)

	w, err := NewWorker(client, "shop")
	require.NoError(t, err)

	h := HandlerFunc(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })

	assert.EqualError(t, w.NewTask("a", h, TaskRetry(step.RetryPolicy{MaxAttempts: 31})),
		`new task "a": retry policy: max attempts must be between 0 and 30, got 31`)
	assert.EqualError(t, w.NewTask("b", h, TaskRetry(step.RetryPolicy{Delays: []time.Duration{-time.Second}})),
		`new task "b": retry policy: delays must not be negative, got -1s`)
	assert.EqualError(t, w.NewTask("c", h, StartDeadline(-time.Second)),
		`new task "c": start deadline must not be negative`)
}

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
				return 0, step.RetryAfter(errors.New("rate limited"), 30*time.Second)
			}, step.Retry(step.RetryPolicy{MaxAttempts: 3, Delays: []time.Duration{time.Second}}))
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
			_, err := step.Do(ctx, "charge", func(context.Context) (int, error) { return 0, step.NonRetriable(declined) })
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
