package runcoil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/sdks/go/pkg/retry"
	"github.com/runcoil/runcoil/sdks/go/pkg/step"
)

type orderInput struct {
	Amount int `json:"amount"`
}

type orderOutput struct {
	Receipt string `json:"receipt"`
}

func TestNewWorkerValidates(t *testing.T) {
	client, err := NewClient(URL("http://localhost:7070"), APIKey("k"))
	require.NoError(t, err)

	tests := []struct {
		name string
		app  string
		opts []WorkerOption
		want string
	}{
		{name: "no app", want: "new worker: app is required"},
		{name: "no slots", app: "shop", opts: []WorkerOption{Slots(0)}, want: "new worker: slots must be at least 1, got 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWorker(client, tt.app, tt.opts...)
			assert.EqualError(t, err, tt.want)
		})
	}

	_, err = NewWorker(nil, "shop")
	assert.Error(t, err)
}

func TestNewTaskValidates(t *testing.T) {
	client, err := NewClient(URL("http://localhost:7070"), APIKey("k"))
	require.NoError(t, err)

	w, err := NewWorker(client, "shop")
	require.NoError(t, err)

	h := HandlerFunc(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })

	assert.EqualError(t, w.NewTask("", h), "new task: name is required")
	assert.EqualError(t, w.NewTask("charge", Handler{}), `new task "charge": handler is required`)
	require.NoError(t, w.NewTask("charge", h))
	assert.EqualError(t, w.NewTask("charge", h), `new task "charge": task name is already registered`)
}

func TestRunSucceeds(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, in orderInput) (orderOutput, error) {
			info, ok := RunInfoFromContext(ctx)
			require.True(t, ok)
			assert.Equal(t, RunInfo{RunID: "run-1", App: "shop", TaskName: "order"}, info)

			charged, err := step.Do(ctx, "charge", func(context.Context) (int, error) { return in.Amount, nil })
			if err != nil {
				return orderOutput{}, err
			}

			receipt, err := step.Do(ctx, "receipt", func(context.Context) (string, error) { return "r-42", nil })
			if err != nil {
				return orderOutput{}, err
			}

			assert.Equal(t, 30, charged)

			return orderOutput{Receipt: receipt}, nil
		}),
	})

	st.send <- assignMsg("run-1", 7, "order", `{"amount":30}`)

	run := f.waitRun(t)
	assert.Equal(t, &runcoilv1.Lease{RunId: "run-1", Epoch: 7}, run.GetLease())
	assert.JSONEq(t, `{"receipt":"r-42"}`, string(run.GetOutput()))

	registers, _, listenAuth, steps := f.snapshot()
	assert.Equal(t, []string{testAPIKey}, registers, "Register sends the API key")
	assert.Equal(t, []string{testSession}, listenAuth, "Listen sends the session token")

	require.Len(t, steps, 2)
	assert.Equal(t, "charge", steps[0].GetStepId())
	assert.Equal(t, "30", string(steps[0].GetOutput()))
	assert.Equal(t, "receipt", steps[1].GetStepId())
	assert.NotNil(t, steps[0].GetStartedAt())
}

func TestReplayReturnsTheStoredStep(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	var charged atomic.Int32
	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			id, err := step.Do(ctx, "charge", func(context.Context) (string, error) {
				charged.Add(1)
				return "new", nil
			})
			if err != nil {
				return orderOutput{}, err
			}

			return orderOutput{Receipt: id}, nil
		}),
	})

	stored := &runcoilv1.StepRecord{
		StepId: "charge",
		Kind:   runcoilv1.StepRecord_KIND_DO,
		Result: &runcoilv1.StepRecord_Output{Output: []byte(`"stored"`)},
	}
	st.send <- assignMsg("run-1", 1, "order", `{}`, stored)

	run := f.waitRun(t)
	assert.JSONEq(t, `{"receipt":"stored"}`, string(run.GetOutput()))
	assert.Zero(t, charged.Load(), "a stored Step does not run again")

	_, _, _, steps := f.snapshot()
	assert.Empty(t, steps)
}

func TestAStepFailureFailsTheRun(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			_, err := step.Do(ctx, "charge", func(context.Context) (string, error) {
				return "", errors.New("card declined")
			})

			var stepErr *step.Error
			require.ErrorAs(t, err, &stepErr)
			assert.Equal(t, "charge", stepErr.StepID)

			return orderOutput{}, err
		}),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)

	run := f.waitRun(t)
	assert.Equal(t, runcoilv1.ErrorKind_ERROR_KIND_ERROR, run.GetError().GetKind())
	assert.Equal(t, "card declined", run.GetError().GetMessage())
	assert.Equal(t, "charge", run.GetError().GetStepId())
}

func TestAPanicFailsTheRun(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { panic("boom") }),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)

	run := f.waitRun(t)
	assert.Equal(t, runcoilv1.ErrorKind_ERROR_KIND_PANIC, run.GetError().GetKind())
	assert.Equal(t, "handler panicked: boom", run.GetError().GetMessage())
}

func TestAnUnknownTaskFailsTheRun(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { return orderOutput{}, nil }),
	})

	st.send <- assignMsg("run-1", 1, "refund", `{}`)

	run := f.waitRun(t)
	assert.Equal(t, `task "refund" is not registered by this worker`, run.GetError().GetMessage())
}

func TestLeaseLostStopsTheHandler(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	running := make(chan struct{})
	stopped := make(chan struct{})
	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			_, err := step.Do(ctx, "charge", func(ctx context.Context) (string, error) {
				close(running)
				<-ctx.Done()
				close(stopped)

				return "", ctx.Err()
			})

			return orderOutput{}, err
		}),
	})

	st.send <- assignMsg("run-1", 3, "order", `{}`)
	<-running

	st.send <- leaseLostMsg("run-1", 3)

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the handler's context was not cancelled")
	}

	f.noRun(t)

	_, _, _, steps := f.snapshot()
	assert.Empty(t, steps, "a Step of a lost attempt is not reported")
}

func TestReconnectReportsTheRunningAttempts(t *testing.T) {
	f, url := newFakeServer(t)
	first, second := newFakeStream(), newFakeStream()
	f.streams <- first
	f.streams <- second

	release := make(chan struct{})
	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			<-release
			return orderOutput{Receipt: "done"}, nil
		}),
	})

	first.send <- assignMsg("run-1", 5, "order", `{}`)
	close(first.send)

	require.Eventually(t, func() bool {
		_, listens, _, _ := f.snapshot()
		return len(listens) == 2
	}, 5*time.Second, 10*time.Millisecond)

	_, listens, _, _ := f.snapshot()
	assert.Empty(t, listens[0].GetRunning())
	require.Len(t, listens[1].GetRunning(), 1)
	assert.Equal(t, &runcoilv1.Lease{RunId: "run-1", Epoch: 5}, listens[1].GetRunning()[0])

	close(release)

	run := f.waitRun(t)
	assert.JSONEq(t, `{"receipt":"done"}`, string(run.GetOutput()))
}

func TestAnEndedSessionRegistersAgain(t *testing.T) {
	f, url := newFakeServer(t)
	f.streams <- &fakeStream{refuse: codedError(t, connect.CodeUnauthenticated, runcoilv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "worker session has ended, register again")}
	f.streams <- newFakeStream()

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { return orderOutput{}, nil }),
	})

	require.Eventually(t, func() bool {
		registers, listens, _, _ := f.snapshot()
		return len(registers) == 2 && len(listens) == 2
	}, 5*time.Second, 10*time.Millisecond)
}

func TestShutdownWaitsForTheHandlers(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	running := make(chan struct{})
	w := startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) {
			close(running)
			time.Sleep(100 * time.Millisecond)

			return orderOutput{Receipt: "done"}, nil
		}),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)
	<-running

	require.NoError(t, w.Shutdown(context.Background()))

	run := f.waitRun(t)
	assert.JSONEq(t, `{"receipt":"done"}`, string(run.GetOutput()))

	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, []string{"shutdown"}, f.deregisters)
	assert.Equal(t, []int{1}, f.runsAtDeregister, "Deregister comes after the result")
}

func TestShutdownGivesUpAtItsDeadline(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	running := make(chan struct{})
	w := startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			close(running)
			<-ctx.Done()

			return orderOutput{}, ctx.Err()
		}),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)
	<-running

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := w.Shutdown(ctx)
	assert.EqualError(t, err, "shutdown: gave up 1 runs before their result was sent: [run-1]")

	f.noRun(t)

	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, []string{"shutdown"}, f.deregisters, "a drain that gave up still deregisters")
}

func TestStartFailsWhenRegisterIsRefused(t *testing.T) {
	client, err := NewClient(URL("http://127.0.0.1:1"), APIKey("k"))
	require.NoError(t, err)

	w, err := NewWorker(client, "shop")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	assert.ErrorContains(t, w.Start(ctx), "start worker: register:")
	assert.ErrorContains(t, w.Start(ctx), "start worker: register:", "a failed Start can be tried again")
}

func TestARefusedRegisterNamesTheTask(t *testing.T) {
	f, url := newFakeServer(t)
	detail, err := connectproto.NewErrorDetail(&runcoilv1.ErrorDetail{
		Code:     runcoilv1.ErrorCode_ERROR_CODE_INVALID_REGISTRATION,
		TaskName: "cleanup",
	})
	require.NoError(t, err)
	f.set(func(f *fakeServer) {
		f.regAnswer = connect.NewError(connect.CodeInvalidArgument, "schedule: cron never fires").WithDetail(detail)
	})

	client, err := NewClient(URL(url), APIKey(testAPIKey))
	require.NoError(t, err)
	w, err := NewWorker(client, "shop")
	require.NoError(t, err)
	require.NoError(t, w.NewTask("cleanup", HandlerFunc(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil }),
		Cron("0 0 30 2 *")))

	assert.EqualError(t, w.Start(context.Background()),
		`start worker: register: task "cleanup": invalid_argument: schedule: cron never fires`)
}

func TestRetryable(t *testing.T) {
	assert.True(t, retryable(connect.NewError(connect.CodeUnavailable, "down")))
	assert.False(t, retryable(connect.NewError(connect.CodeInvalidArgument, "bad")))
	assert.False(t, retryable(codedError(t, connect.CodeFailedPrecondition, runcoilv1.ErrorCode_ERROR_CODE_STALE_LEASE, "stale")))
}

func TestReconnectReportsAnAttemptWhoseResultIsBeingSent(t *testing.T) {
	f, url := newFakeServer(t)
	first, second := newFakeStream(), newFakeStream()
	f.streams <- first
	f.streams <- second

	release := make(chan struct{})
	sending := make(chan struct{}, 1)
	f.set(func(f *fakeServer) {
		f.runAnswer = func(ctx context.Context) error {
			sending <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { return orderOutput{Receipt: "done"}, nil }),
	})

	first.send <- assignMsg("run-1", 4, "order", `{}`)
	<-sending
	close(first.send)

	require.Eventually(t, func() bool {
		_, listens, _, _ := f.snapshot()
		return len(listens) == 2
	}, 5*time.Second, 10*time.Millisecond)

	_, listens, _, _ := f.snapshot()
	require.Len(t, listens[1].GetRunning(), 1, "an attempt whose result is being sent keeps its lease")
	assert.Equal(t, &runcoilv1.Lease{RunId: "run-1", Epoch: 4}, listens[1].GetRunning()[0])

	close(release)
	f.waitRun(t)
}

func TestARefusedHeartbeatRegistersAgain(t *testing.T) {
	f, url := newFakeServer(t)
	first, second := newFakeStream(), newFakeStream()
	f.streams <- first
	f.streams <- second

	ended := codedError(t, connect.CodeUnauthenticated, runcoilv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "invalid session token")
	f.set(func(f *fakeServer) {
		f.hbInterval = 50 * time.Millisecond
		f.beatAnswer = func() error {
			f.mu.Lock()
			defer f.mu.Unlock()

			if f.sessions < 2 {
				return ended
			}

			return nil
		}
	})

	// The server keeps a stream open after its session ends, with keepalives.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		keepalive := &runcoilv1.ListenResponse{Kind: &runcoilv1.ListenResponse_Keepalive{Keepalive: &runcoilv1.Keepalive{}}}
		for {
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
				select {
				case first.send <- keepalive:
				default:
				}
			}
		}
	}()

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { return orderOutput{}, nil }),
	})

	require.Eventually(t, func() bool {
		registers, listens, _, _ := f.snapshot()
		return len(registers) == 2 && len(listens) == 2
	}, 5*time.Second, 10*time.Millisecond)

	_, _, listenAuth, _ := f.snapshot()
	assert.Equal(t, []string{"session-1", "session-2"}, listenAuth, "the new Listen sends the new session token")
}

func TestAStaleStepEndsTheAttempt(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	stale := codedError(t, connect.CodeFailedPrecondition, runcoilv1.ErrorCode_ERROR_CODE_STALE_LEASE, "lease is no longer this attempt's")
	f.set(func(f *fakeServer) {
		f.stepAnswer = func(*runcoilv1.CompleteStepRequest) (*runcoilv1.CompleteStepResponse, error) { return nil, stale }
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

	f.noRun(t)
	assert.False(t, after.Load(), "the handler does not run past a refused Step")

	_, _, _, steps := f.snapshot()
	assert.Len(t, steps, 1, "a stale Step report is not sent again")
}

func TestAssignsOfTheSameRun(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	var calls atomic.Int32
	firstRunning := make(chan struct{})
	firstEnded := make(chan struct{})
	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
			if calls.Add(1) == 1 {
				close(firstRunning)
				<-ctx.Done()
				close(firstEnded)

				return orderOutput{}, ctx.Err()
			}

			return orderOutput{Receipt: "second"}, nil
		}),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)
	<-firstRunning

	st.send <- assignMsg("run-1", 1, "order", `{}`)
	st.send <- assignMsg("run-1", 2, "order", `{}`)

	select {
	case <-firstEnded:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "a newer epoch did not end the held attempt")
	}

	run := f.waitRun(t)
	assert.Equal(t, int64(2), run.GetLease().GetEpoch())
	assert.Equal(t, int32(2), calls.Load(), "the repeated Assign of epoch 1 is ignored")
	f.noRun(t)
}

func TestShutdownCutsAResultStillBeingSent(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	sending := make(chan struct{}, 1)
	f.set(func(f *fakeServer) {
		f.runAnswer = func(ctx context.Context) error {
			sending <- struct{}{}
			<-ctx.Done()

			return ctx.Err()
		}
	})

	w := startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { return orderOutput{}, nil }),
	})

	st.send <- assignMsg("run-1", 1, "order", `{}`)
	<-sending

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := w.Shutdown(ctx)

	assert.EqualError(t, err, "shutdown: gave up 1 runs before their result was sent: [run-1]")
	assert.Less(t, time.Since(start), 3*handlerWait, "Shutdown does not wait for the report's whole budget")
}

func TestAnEndedAttemptNeverStartsItsHandler(t *testing.T) {
	client, err := NewClient(URL("http://localhost:7070"), APIKey("k"))
	require.NoError(t, err)

	for range 50 {
		w, err := NewWorker(client, "shop")
		require.NoError(t, err)

		var called atomic.Bool
		h := HandlerFunc(func(context.Context, orderInput) (orderOutput, error) {
			called.Store(true)
			return orderOutput{}, nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		a := &attempt{runID: "run-1", epoch: 1, logger: w.logger, ctx: ctx, cancel: cancel}
		a.end()

		w.running.Add(1)
		w.execute(a, &task{name: "order", handler: h}, &runcoilv1.Assign{TaskName: "order"})

		require.False(t, called.Load(), "the handler of an ended attempt ran")
		require.Empty(t, w.slots, "the slot is given back")
	}
}

func TestARefusedReportRegistersAgain(t *testing.T) {
	ended := codedError(t, connect.CodeUnauthenticated, runcoilv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED, "invalid session token")

	tests := []struct {
		name    string
		refuse  func(f *fakeServer)
		handler Handler
	}{
		{
			name: "CompleteStep",
			refuse: func(f *fakeServer) {
				f.stepAnswer = func(*runcoilv1.CompleteStepRequest) (*runcoilv1.CompleteStepResponse, error) { return nil, ended }
			},
			handler: HandlerFunc(func(ctx context.Context, _ orderInput) (orderOutput, error) {
				_, err := step.Do(ctx, "charge", func(context.Context) (int, error) { return 1, nil })
				return orderOutput{}, err
			}),
		},
		{
			name: "CompleteRun",
			refuse: func(f *fakeServer) {
				f.runAnswer = func(context.Context) error { return ended }
			},
			handler: HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { return orderOutput{}, nil }),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, url := newFakeServer(t)
			first := newFakeStream()
			f.streams <- first
			f.streams <- newFakeStream()
			f.set(tt.refuse)

			startWorker(t, url, map[string]Handler{"order": tt.handler})

			first.send <- assignMsg("run-1", 1, "order", `{}`)

			require.Eventually(t, func() bool {
				registers, listens, _, _ := f.snapshot()
				return len(registers) == 2 && len(listens) == 2
			}, 5*time.Second, 10*time.Millisecond)

			_, _, listenAuth, _ := f.snapshot()
			assert.Equal(t, []string{"session-1", "session-2"}, listenAuth)
		})
	}
}

func TestASilentStreamIsDropped(t *testing.T) {
	f, url := newFakeServer(t)
	f.streams <- newFakeStream()
	f.streams <- newFakeStream()
	f.set(func(f *fakeServer) { f.hbInterval = 50 * time.Millisecond })

	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) { return orderOutput{}, nil }),
	})

	require.Eventually(t, func() bool {
		_, listens, _, _ := f.snapshot()
		return len(listens) == 2
	}, 5*time.Second, 10*time.Millisecond, "a stream with no keepalive for three intervals is opened again")

	registers, _, _, _ := f.snapshot()
	assert.Len(t, registers, 1, "a silent stream is not an ended session")
}

func TestTheWorkerRunsAtMostItsSlots(t *testing.T) {
	f, url := newFakeServer(t)
	st := newFakeStream()
	f.streams <- st

	var running, most atomic.Int32
	release := make(chan struct{})
	startWorker(t, url, map[string]Handler{
		"order": HandlerFunc(func(context.Context, orderInput) (orderOutput, error) {
			n := running.Add(1)
			for {
				m := most.Load()
				if n <= m || most.CompareAndSwap(m, n) {
					break
				}
			}

			<-release
			running.Add(-1)

			return orderOutput{}, nil
		}),
	})

	for _, id := range []string{"run-1", "run-2", "run-3"} {
		st.send <- assignMsg(id, 1, "order", `{}`)
	}

	require.Eventually(t, func() bool { return running.Load() == 2 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(2), running.Load(), "the third Run waits for a slot")

	close(release)
	for range 3 {
		f.waitRun(t)
	}

	assert.Equal(t, int32(2), most.Load())
}

func TestRegisterCarriesTheTaskOptions(t *testing.T) {
	f, url := newFakeServer(t)
	f.streams <- newFakeStream()

	client, err := NewClient(URL(url), APIKey(testAPIKey))
	require.NoError(t, err)

	w, err := NewWorker(client, "shop")
	require.NoError(t, err)

	h := HandlerFunc(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })
	require.NoError(t, w.NewTask("charge", h,
		Retry(retry.Policy{MaxAttempts: 5, Delays: []time.Duration{time.Second, time.Minute}}),
		StartDeadline(10*time.Minute),
		Cron("0 2 * * *", SkipOverlap()),
	))
	require.NoError(t, w.NewTask("plain", h, Cron("*/10 * * * * *")))

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
	assert.Equal(t, "0 2 * * *", defs[0].GetSchedule().GetCron())
	assert.Equal(t, runcoilv1.OverlapPolicy_OVERLAP_POLICY_SKIP, defs[0].GetSchedule().GetOverlap())
	assert.Nil(t, defs[1].GetRetry(), "no policy: the server's default")
	assert.Nil(t, defs[1].GetStartDeadline(), "no deadline")
	assert.Equal(t, "*/10 * * * * *", defs[1].GetSchedule().GetCron())
	assert.Equal(t, runcoilv1.OverlapPolicy_OVERLAP_POLICY_ALLOW, defs[1].GetSchedule().GetOverlap())
}

func TestNewTaskValidatesItsOptions(t *testing.T) {
	client, err := NewClient(URL("http://localhost:7070"), APIKey("k"))
	require.NoError(t, err)

	w, err := NewWorker(client, "shop")
	require.NoError(t, err)

	h := HandlerFunc(func(context.Context, struct{}) (struct{}, error) { return struct{}{}, nil })

	assert.EqualError(t, w.NewTask("a", h, Retry(retry.Policy{MaxAttempts: 31})),
		`new task "a": retry policy: max attempts must be between 0 and 30, got 31`)
	assert.EqualError(t, w.NewTask("b", h, Retry(retry.Policy{Delays: []time.Duration{-time.Second}})),
		`new task "b": retry policy: delays must not be negative, got -1s`)
	assert.EqualError(t, w.NewTask("c", h, StartDeadline(-time.Second)),
		`new task "c": start deadline must be greater than 0, got -1s`)
	assert.EqualError(t, w.NewTask("d", h, StartDeadline(0)),
		`new task "d": start deadline must be greater than 0, got 0s`)
	assert.EqualError(t, w.NewTask("e", h, Cron("0 * * * *"), Cron("30 * * * *")),
		`new task "e": a task has at most one schedule: declare another task on the same handler for another cron`)
	assert.EqualError(t, w.NewTask("f", h, Cron("  ")),
		`new task "f": cron is empty`)
}
