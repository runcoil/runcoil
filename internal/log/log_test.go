package log

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/pkg/cmdutil"
)

func load(t *testing.T, vars map[string]string) (Config, error) {
	t.Helper()

	var cfg Config
	err := cmdutil.Load("test", &cfg, nil, func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	})

	return cfg, err
}

func TestConfigDefaults(t *testing.T) {
	cfg, err := load(t, nil)

	require.NoError(t, err)
	assert.Equal(t, Config{Level: slog.LevelInfo, Format: FormatText}, cfg)
	assert.NoError(t, cfg.Validate())
}

func TestConfigValues(t *testing.T) {
	cfg, err := load(t, map[string]string{"LOG_LEVEL": "debug", "LOG_FORMAT": "JSON"})

	require.NoError(t, err)
	assert.Equal(t, Config{Level: slog.LevelDebug, Format: FormatJSON}, cfg)
}

func TestConfigBadValues(t *testing.T) {
	_, err := load(t, map[string]string{"LOG_LEVEL": "loud", "LOG_FORMAT": "xml"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOG_LEVEL: log.level")
	assert.Contains(t, err.Error(), "LOG_FORMAT: log.format")
}

func TestValidate(t *testing.T) {
	cfg := Config{Format: Format(7)}

	assert.EqualError(t, cfg.Validate(), "log.format must be text or json")
}

func TestFormatText(t *testing.T) {
	b, err := FormatJSON.MarshalText()

	require.NoError(t, err)
	assert.Equal(t, "json", string(b))
	assert.Equal(t, "text", FormatText.String())
}
