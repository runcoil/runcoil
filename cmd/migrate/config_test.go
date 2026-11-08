package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/pkg/configutil"
)

func defaults(t *testing.T) Config {
	t.Helper()

	var cfg Config
	require.NoError(t, configutil.Load("test", &cfg, nil, func(string) (string, bool) { return "", false }))

	return cfg
}

func TestDefaultsAreValid(t *testing.T) {
	cfg := defaults(t)

	assert.NoError(t, cfg.Validate())
}

func TestValidateNamesTheSection(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
		want   string
	}{
		{name: "log", modify: func(c *Config) { c.Log.Format = "xml" }, want: "log: format must be text or json"},
		{name: "postgres", modify: func(c *Config) { c.Postgres.Port = 0 }, want: "postgres: port must be between 1 and 65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaults(t)
			tt.modify(&cfg)

			assert.EqualError(t, cfg.Validate(), tt.want)
		})
	}
}
