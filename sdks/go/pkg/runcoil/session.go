package runcoil

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect/v2"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/proto/gen/go/runcoil/v1/runcoilv1connect"
)

const (
	registerTimeout = 10 * time.Second // bounds Register, and every call before the server has answered it
	minCallTimeout  = time.Second      // floor for unary calls after registration
)

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
		defs = append(defs, taskDefinitionToProto(t))
	}

	return &runcoilv1.RegisterRequest{App: w.app, Tasks: defs, Slots: w.slotCount}
}

// register opens a worker session and keeps its token and heartbeat interval.
func (w *Worker) register(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, registerTimeout)
	defer cancel()

	req := w.registerRequest()
	resp, err := w.rpc.Register(ctx, req)
	if err != nil {
		if name := errorDetail(err).GetTaskName(); name != "" {
			return fmt.Errorf("register: task %q: %w", name, err)
		}

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

	w.sessionEnded.Store(false)

	tasks := make([]string, 0, len(req.GetTasks()))
	for _, t := range req.GetTasks() {
		tasks = append(tasks, t.GetName())
	}

	w.logger.InfoContext(ctx, "worker registered",
		slog.String("app", w.app),
		slog.String("worker_id", resp.GetWorkerId()),
		slog.Int("slots", int(req.GetSlots())),
		slog.Any("tasks", tasks),
	)

	return nil
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

// heartbeat tells the server the worker process is alive.
func (w *Worker) heartbeat(ctx context.Context) {
	callCtx, cancel := context.WithTimeout(ctx, w.callTimeout())
	defer cancel()

	session := w.sessionToken()
	if _, err := w.rpc.Heartbeat(callCtx, &runcoilv1.HeartbeatRequest{}); err != nil {
		if w.endSession(ctx, session, err) || ctx.Err() != nil {
			return
		}

		w.conn.failed(ctx, err, w.heartbeatInterval())

		return
	}

	w.conn.ok(ctx)
}

// errSessionEnded is what listenOnce returns when it ended its stream because the session
// ended.
var errSessionEnded = errors.New("worker session ended")

// endSession handles an answer to a call made with session. When the server says the
// session has ended, it ends the open Listen stream, so the listen loop registers again,
// and reports true. The server keeps a stream open after its session ends, so a call
// such as a heartbeat may be the first to learn it.
func (w *Worker) endSession(ctx context.Context, session string, err error) bool {
	if errorCode(err) != runcoilv1.ErrorCode_ERROR_CODE_UNAUTHENTICATED {
		return false
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// A session that registering again has already replaced ends nothing.
	if session != w.session {
		return true
	}

	w.logger.InfoContext(ctx, "worker session ended; registering again", slog.Any("error", err))
	w.sessionEnded.Store(true)
	if w.streamCancel != nil {
		w.streamCancel()
	}

	return true
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
