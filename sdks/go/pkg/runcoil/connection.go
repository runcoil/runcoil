package runcoil

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// reminderEvery is how often an outage repeats its warning while it lasts.
const reminderEvery = time.Minute

// connection follows the state of the worker's connection to the server, not each
// attempt to reach it. The first failure of an outage is one warning; the failed
// attempts after it are debug lines; a reminder repeats the warning on a fixed interval;
// the first success after the outage is one info line. It is safe for concurrent use by
// the listen loop, the heartbeat loop, and the attempt goroutines.
type connection struct {
	logger *slog.Logger
	app    string

	mu       sync.Mutex
	down     bool
	since    time.Time
	attempts int
	lastErr  error
	gen      uint64 // identifies the current outage; a timer of an older one does nothing

	// answered signals that the server has answered: the listen loop's wait ends early.
	answered chan struct{}
}

// failed records a failed attempt to reach the server. next is the wait before the next
// attempt. The first failure of an outage is logged as a warning.
func (c *connection) failed(ctx context.Context, err error, next time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastErr = err
	if !c.down {
		c.down = true
		c.since = time.Now()
		c.attempts = 1
		c.gen++
		c.logger.WarnContext(ctx, "server connection lost",
			slog.Any("error", err),
			slog.String("app", c.app),
		)

		c.armReminder()

		return
	}

	c.attempts++
	c.logger.DebugContext(ctx, "server connection attempt failed",
		slog.Int("attempt", c.attempts),
		slog.Duration("next_delay", next),
		slog.Any("error", err),
	)
}

// ok records that the server answered: the listen stream delivered a message, or a
// heartbeat succeeded. It ends an outage with one info line.
func (c *connection) ok(ctx context.Context) {
	select {
	case c.answered <- struct{}{}:
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.down {
		return
	}

	c.logger.InfoContext(ctx, "server connection restored",
		slog.Duration("outage", time.Since(c.since)),
		slog.Int("attempts", c.attempts),
	)

	c.down = false
	c.attempts = 0
	c.lastErr = nil
	c.gen++
}

// drainAnswered discards an answer that the listen loop has not used yet, so that only
// answers from now on end its next wait.
func (c *connection) drainAnswered() {
	select {
	case <-c.answered:
	default:
	}
}

// stop ends the reminders when the worker shuts down.
func (c *connection) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.down = false
	c.gen++
}

// armReminder schedules the next reminder of the current outage. c.mu is held.
func (c *connection) armReminder() {
	gen := c.gen
	time.AfterFunc(reminderEvery, func() { c.remind(gen) })
}

// remind logs that the outage still lasts, and schedules the next reminder.
func (c *connection) remind(gen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.down || gen != c.gen {
		return
	}

	c.logger.Warn("server still unreachable",
		slog.Duration("down_for", time.Since(c.since)),
		slog.Int("attempts", c.attempts),
		slog.Any("error", c.lastErr),
	)

	c.armReminder()
}
