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

	"github.com/runcoil/runcoil/pkg/cmdutil"
)

func noEnv(string) (string, bool) { return "", false }

func defaults(t *testing.T) Config {
	t.Helper()

	var cfg Config
	require.NoError(t, cmdutil.Load("test", &cfg, nil, noEnv))

	return cfg
}

func TestDefaultsAreValid(t *testing.T) {
	cfg := defaults(t)

	assert.NoError(t, cfg.Validate())
}

func TestValidateReportsEveryError(t *testing.T) {
	cfg := defaults(t)
	cfg.HTTP.Addr = "nope"
	cfg.Postgres.Port = 0
	cfg.Postgres.MaxConns = 0
	cfg.Log.Format = 9

	err := cfg.Validate()

	require.Error(t, err)
	for _, want := range []string{"http.addr", "postgres.port", "postgres.max-conns", "log.format"} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestLoadReportsEveryBadValue(t *testing.T) {
	var cfg Config
	err := cmdutil.Load("test", &cfg, nil, func(key string) (string, bool) {
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
	b, err := os.ReadFile("../../config.example.yml")
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(b, &doc))

	var keys []string
	flatKeys("", doc, &keys)
	slices.Sort(keys)

	assert.Equal(t, cmdutil.FlagNames(&Config{}), keys)

	var cfg Config
	require.NoError(t, cmdutil.Load("test", &cfg, []string{"--config", "../../config.example.yml"}, noEnv))
	assert.Equal(t, defaults(t), cfg, "config.example.yml must hold the defaults")
}

func TestEnvExampleListsEveryFlag(t *testing.T) {
	f, err := os.Open("../../.env.example")
	require.NoError(t, err)
	defer f.Close()

	vars := map[string]string{}
	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, value, ok := strings.Cut(line, "=")
		require.True(t, ok, line)
		vars[name] = value
		names = append(names, name)
	}
	require.NoError(t, scanner.Err())

	var want []string
	for _, flag := range cmdutil.FlagNames(&Config{}) {
		want = append(want, cmdutil.EnvName(flag))
	}
	slices.Sort(want)
	slices.Sort(names)

	assert.Equal(t, want, names)

	var cfg Config
	require.NoError(t, cmdutil.Load("test", &cfg, nil, func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}))
	assert.Equal(t, defaults(t), cfg, ".env.example must hold the defaults")
}
