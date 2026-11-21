package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestScheduledStartDeadline(t *testing.T) {
	tick := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		next time.Time
		want time.Time
	}{
		{"a cron of one second waits at least a minute", tick.Add(time.Second), tick.Add(time.Minute)},
		{"a cron of one minute waits until the next tick", tick.Add(time.Minute), tick.Add(time.Minute)},
		{"a daily cron waits until the next tick", tick.Add(24 * time.Hour), tick.Add(24 * time.Hour)},
		{"a tick fired late counts from the tick, not the sweep", tick.Add(90 * time.Second), tick.Add(90 * time.Second)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, scheduledStartDeadline(tick, tt.next))
		})
	}
}
