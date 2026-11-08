package dispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
)

// streamBuffer is how many messages wait for a stream beyond its slots.
const streamBuffer = 16

// stream is the Listen stream of one worker on this server. The fields after out are
// guarded by Service.mu.
type stream struct {
	id       uuid.UUID
	workerID uuid.UUID
	taskIDs  []uuid.UUID
	slots    int
	out      chan domain.StreamMessage

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
		out:      make(chan domain.StreamMessage, worker.Slots+streamBuffer),
	}

	// The stream goes in the map while the worker row is locked, so the map follows the
	// order of the row lock: a slower, older Listen cannot replace a newer one.
	notHeld, err := s.workerStore.AttachStream(ctx, domain.AttachStreamParams{
		WorkerID:     worker.ID,
		DispatcherID: s.id,
		StreamID:     streamID,
		Running:      running,
		Attached: func(notHeld []domain.Lease) {
			st.free = max(worker.Slots-(len(running)-len(notHeld)), 0)
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
		slog.Int("running", len(running)),
		slog.Int("not_held", len(notHeld)),
	)

	for _, l := range notHeld {
		if err := send(domain.StreamMessage{LeaseLost: &l}); err != nil {
			return err
		}
	}

	keepalive := time.NewTicker(s.cfg.HeartbeatInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.stopped:
			return nil
		case msg := <-st.out:
			if err := send(msg); err != nil {
				return err
			}
		case <-keepalive.C:
			if err := send(domain.StreamMessage{Keepalive: true}); err != nil {
				return err
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

// push sends the claimed attempts to the stream that claimed them. An attempt that cannot
// be sent keeps its lease: the worker does not report it at its next Listen, so the
// reconcile puts it back in the queue.
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
		default:
			s.logger.WarnContext(ctx, "stream buffer full",
				slog.String("worker_id", workerID.String()),
				slog.String("run_id", a.Lease.RunID.String()),
			)
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

func attachError(err error) error {
	if err == nil {
		return nil
	}

	if isNotFound(err) {
		return apierr.Unauthenticated(apierr.CodeUnauthenticated, "worker session has ended, register again")
	}

	return err
}
