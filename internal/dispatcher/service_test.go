package dispatcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWorkerConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  WorkerConfig
		want string
	}{
		{name: "defaults", cfg: WorkerConfig{HeartbeatInterval: 10 * time.Second, TTL: 30 * time.Second}},
		{name: "no interval", cfg: WorkerConfig{TTL: time.Second}, want: "heartbeat interval must be greater than 0"},
		{name: "ttl too short", cfg: WorkerConfig{HeartbeatInterval: time.Second, TTL: time.Second}, want: "ttl must be longer than the heartbeat interval"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}

			assert.EqualError(t, err, tt.want)
		})
	}
}
