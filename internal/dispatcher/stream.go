package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

// streamBuffer is how many messages wait for a stream: an Assign for every slot a worker
// may have, and room for the other messages.
const streamBuffer = domain.MaxSlots + 16

// stream is the Listen stream of one worker on this server. The fields after ended are
// guarded by Service.mu.
type stream struct {
	id       uuid.UUID
	workerID uuid.UUID
	taskIDs  []uuid.UUID
	slots    int
	out      chan domain.StreamMessage
	// ended is closed when the dispatcher ends the stream, so its Listen returns.
	ended chan struct{}

	free int
}

// claimTarget is a stream with free slots, copied out of the map for one claim.
type claimTarget struct {
	workerID uuid.UUID
	streamID uuid.UUID
	taskIDs  []uuid.UUID
	free     int
}

// Listen attaches the worker to a new stream and sends its messages until ctx ends or
// the dispatcher stops. running is what the worker reports it is running: the leases it
// does not report go back to the queue, and the reported attempts it no longer holds get
// a lease-lost message.
func (s *Service) Listen(
	ctx context.Context,
	worker domain.Worker,
	running []domain.Lease,
	send func(domain.StreamMessage) error,
) error {
	if s.isStopping() {
		return apierr.Unavailable("", "server is shutting down")
	}

	streamID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("new stream id: %w", err)
	}

	st := &stream{
		id:       streamID,
		workerID: worker.ID,
		taskIDs:  worker.TaskIDs,
		slots:    worker.Slots,
		out:      make(chan domain.StreamMessage, streamBuffer),
		ended:    make(chan struct{}),
	}

	// The stream goes in the map while the worker row is locked, so the map follows the
	// order of the row lock: a slower, older Listen cannot replace a newer one.
	free := 0
	notHeld, err := s.workerStore.AttachStream(ctx, domain.AttachStreamParams{
		WorkerID:     worker.ID,
		DispatcherID: s.id,
		StreamID:     streamID,
		Running:      running,
		Attached: func(notHeld []domain.Lease) {
			free = max(worker.Slots-(len(running)-len(notHeld)), 0)
			st.free = free
			s.put(st)
		},
	})
	if err != nil {
		s.drop(worker.ID, streamID)
		return attachError(err)
	}
	defer s.drop(worker.ID, streamID)

	s.logger.InfoContext(ctx, "stream attached",
		slog.String("worker_id", worker.ID.String()),
		slog.String("stream_id", streamID.String()),
		slog.String("app", worker.App),
		slog.Int("running", len(running)),
		slog.Int("not_held", len(notHeld)),
		slog.Int("free_slots", free),
	)

	attachedAt := time.Now()
	reason := "worker closed"
	defer func() {
		s.logger.InfoContext(ctx, "stream detached",
			slog.String("worker_id", worker.ID.String()),
			slog.String("stream_id", streamID.String()),
			slog.String("reason", reason),
			slog.Duration("duration", time.Since(attachedAt)),
		)
	}()

	for _, l := range notHeld {
		s.logger.InfoContext(ctx, "lease lost sent",
			slog.String("worker_id", worker.ID.String()),
			slog.String("run_id", l.RunID.String()),
			slog.Int64("epoch", l.Epoch),
		)

		if err := send(domain.StreamMessage{LeaseLost: &l}); err != nil {
			reason = "send failed"
			return fmt.Errorf("send lease lost: %w", err)
		}
	}

	keepalive := time.NewTicker(s.cfg.HeartbeatInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.stopped:
			reason = "server stopping"
			return nil
		case <-st.ended:
			reason = "ended by the dispatcher"
			return errStreamEnded
		case msg := <-st.out:
			if err := send(msg); err != nil {
				reason = "send failed"
				return fmt.Errorf("send message: %w", err)
			}
		case <-keepalive.C:
			if err := send(domain.StreamMessage{Keepalive: true}); err != nil {
				reason = "send failed"
				return fmt.Errorf("send keepalive: %w", err)
			}
		}
	}
}

// put makes st the worker's stream on this server.
func (s *Service) put(st *stream) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.streams[st.workerID] = st
}

// drop removes the worker's stream if it is still streamID, so an old stream that ends
// late never removes a newer one.
func (s *Service) drop(workerID, streamID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if st, ok := s.streams[workerID]; ok && st.id == streamID {
		delete(s.streams, workerID)
	}
}

// end removes the worker's stream if it is still streamID and ends its Listen. The worker
// reconnects, and the reconcile of its next Listen requeues the attempts it was never
// sent.
func (s *Service) end(workerID, streamID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if st, ok := s.streams[workerID]; ok && st.id == streamID {
		s.endLocked(st)
	}
}

// endLocked is end for a caller that holds s.mu.
func (s *Service) endLocked(st *stream) {
	delete(s.streams, st.workerID)
	close(st.ended)
}

// claimable returns the streams with free slots.
func (s *Service) claimable() []claimTarget {
	s.mu.Lock()
	defer s.mu.Unlock()

	targets := make([]claimTarget, 0, len(s.streams))
	for _, st := range s.streams {
		if st.free > 0 {
			targets = append(targets, claimTarget{
				workerID: st.workerID,
				streamID: st.id,
				taskIDs:  st.taskIDs,
				free:     st.free,
			})
		}
	}

	return targets
}

// push sends the claimed attempts to the stream that claimed them. When one cannot be
// sent, the stream ends: the worker reconnects without reporting it, so the reconcile
// puts it back in the queue.
func (s *Service) push(ctx context.Context, workerID, streamID uuid.UUID, assigns []domain.Assign) {
	if len(assigns) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.streams[workerID]
	if !ok || st.id != streamID {
		s.logger.WarnContext(ctx, "stream gone before assign",
			slog.String("worker_id", workerID.String()),
			slog.Int("runs", len(assigns)),
		)

		return
	}

	st.free -= len(assigns)
	for _, a := range assigns {
		select {
		case st.out <- domain.StreamMessage{Assign: &a}:
			s.logger.DebugContext(ctx, "run assigned",
				slog.String("run_id", a.Lease.RunID.String()),
				slog.Int64("epoch", a.Lease.Epoch),
				slog.String("task_name", a.TaskName),
				slog.String("worker_id", workerID.String()),
			)
		default:
			s.logger.WarnContext(ctx, "stream buffer full, ending the stream",
				slog.String("worker_id", workerID.String()),
				slog.String("run_id", a.Lease.RunID.String()),
			)
			s.endLocked(st)

			return
		}
	}
}

// release gives back the slot of an attempt that ended.
func (s *Service) release(workerID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if st, ok := s.streams[workerID]; ok && st.free < st.slots {
		st.free++
	}
}

var errStreamEnded = apierr.Unavailable("", "stream ended, reconnect")

func attachError(err error) error {
	if errors.Is(err, domain.ErrSessionEnded) {
		return apierr.Unauthenticated(apierr.CodeUnauthenticated, "worker session has ended, register again")
	}

	return fmt.Errorf("attach stream: %w", err)
}
