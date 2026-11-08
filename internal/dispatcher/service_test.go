package dispatcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "defaults", cfg: Config{PollInterval: time.Second, HeartbeatInterval: 10 * time.Second, WorkerTTL: 30 * time.Second}},
		{name: "no poll interval", cfg: Config{HeartbeatInterval: time.Second, WorkerTTL: 2 * time.Second}, want: "poll interval must be greater than 0"},
		{name: "no heartbeat interval", cfg: Config{PollInterval: time.Second, WorkerTTL: time.Second}, want: "heartbeat interval must be greater than 0"},
		{name: "ttl too short", cfg: Config{PollInterval: time.Second, HeartbeatInterval: time.Second, WorkerTTL: time.Second}, want: "worker ttl must be longer than the heartbeat interval"},
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
