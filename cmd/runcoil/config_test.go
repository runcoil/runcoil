package main

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/runcoil/runcoil/pkg/configutil"
)

func noEnv(string) (string, bool) { return "", false }

func defaults(t *testing.T) Config {
	t.Helper()

	var cfg Config
	require.NoError(t, configutil.Load("test", &cfg, nil, noEnv))

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
		{name: "http", modify: func(c *Config) { c.HTTP.Port = 0 }, want: "http: port must be between 1 and 65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaults(t)
			tt.modify(&cfg)

			assert.EqualError(t, cfg.Validate(), tt.want)
		})
	}
}

func TestLoadReportsEveryBadValue(t *testing.T) {
	var cfg Config
	err := configutil.Load("test", &cfg, nil, func(key string) (string, bool) {
		v, ok := map[string]string{"POSTGRES_PORT": "tcp://x", "LOG_LEVEL": "loud"}[key]
		return v, ok
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "POSTGRES_PORT: postgres.port")
	assert.Contains(t, err.Error(), "LOG_LEVEL: log.level")
}

func flatKeys(prefix string, in map[string]any, out *[]string) {
	for k, v := range in {
		key := strings.ReplaceAll(prefix+k, "_", "-")
		if m, ok := v.(map[string]any); ok {
			flatKeys(key+".", m, out)
			continue
		}
		*out = append(*out, key)
	}
}

func TestConfigExampleListsEveryFlag(t *testing.T) {
	b, err := os.ReadFile("config.example.yml")
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(b, &doc))

	var keys []string
	flatKeys("", doc, &keys)
	slices.Sort(keys)

	assert.Equal(t, configutil.FlagNames(&Config{}), keys)

	var cfg Config
	require.NoError(t, configutil.Load("test", &cfg, []string{"--config", "config.example.yml"}, noEnv))
	assert.Equal(t, defaults(t), cfg, "config.example.yml must hold the defaults")
}

func TestEnvExampleListsEveryFlag(t *testing.T) {
	f, err := os.Open(".env.example")
	require.NoError(t, err)
	defer f.Close()

	vars := map[string]string{}
	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		name, value, ok := strings.Cut(line, "=")
		require.True(t, ok, line)
		vars[name] = value
		names = append(names, name)
	}
	require.NoError(t, scanner.Err())

	var want []string
	for _, flag := range configutil.FlagNames(&Config{}) {
		want = append(want, configutil.EnvName(flag))
	}
	slices.Sort(want)
	slices.Sort(names)

	assert.Equal(t, want, names)

	var cfg Config
	require.NoError(t, configutil.Load("test", &cfg, nil, func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}))
	assert.Equal(t, defaults(t), cfg, ".env.example must hold the defaults")
}
