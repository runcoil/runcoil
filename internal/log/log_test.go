package log

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/pkg/configutil"
)

func load(t *testing.T, vars map[string]string) (Config, error) {
	t.Helper()

	var cfg Config
	err := configutil.Load("test", &cfg, nil, func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	})

	return cfg, err
}

func TestConfigDefaults(t *testing.T) {
	cfg, err := load(t, nil)

	require.NoError(t, err)
	assert.Equal(t, Config{Level: slog.LevelInfo, Format: "text"}, cfg)
	assert.NoError(t, cfg.Validate())
}

func TestConfigValues(t *testing.T) {
	cfg, err := load(t, map[string]string{"LOG_LEVEL": "debug", "LOG_FORMAT": "json"})

	require.NoError(t, err)
	assert.Equal(t, Config{Level: slog.LevelDebug, Format: "json"}, cfg)
}

func TestConfigBadLevel(t *testing.T) {
	_, err := load(t, map[string]string{"LOG_LEVEL": "loud"})

	assert.ErrorContains(t, err, "LOG_LEVEL: log.level")
}

func TestValidate(t *testing.T) {
	cfg := Config{Format: "xml"}

	assert.EqualError(t, cfg.Validate(), "format must be text or json")
}
