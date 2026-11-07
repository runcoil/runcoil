package configutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	Host     string
	Port     int
	MaxConns int
	Delays   []time.Duration
}

func (c *testConfig) RegisterFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Host, "db.host", "localhost", "")
	fs.IntVar(&c.Port, "db.port", 5432, "")
	fs.IntVar(&c.MaxConns, "db.max-conns", 5, "")
	fs.DurationSliceVar(&c.Delays, "retry.delays", []time.Duration{time.Second}, "")
}

type collidingConfig struct {
	testConfig
	other int
}

func (c *collidingConfig) RegisterFlags(fs *pflag.FlagSet) {
	c.testConfig.RegisterFlags(fs)
	fs.IntVar(&c.other, "db-max.conns", 1, "")
}

func env(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoadDefaults(t *testing.T) {
	var cfg testConfig
	require.NoError(t, Load("test", &cfg, nil, env(nil)))

	assert.Equal(t, testConfig{Host: "localhost", Port: 5432, MaxConns: 5, Delays: []time.Duration{time.Second}}, cfg)
}

func TestLoadSourceOrder(t *testing.T) {
	path := writeFile(t, `
db:
  host: file-host
  port: 6000
  max_conns: 7
retry:
  delays: [5s, 30s]
`)

	tests := []struct {
		name string
		args []string
		env  map[string]string
		want testConfig
	}{
		{
			name: "file over default",
			args: []string{"--config", path},
			want: testConfig{Host: "file-host", Port: 6000, MaxConns: 7, Delays: []time.Duration{5 * time.Second, 30 * time.Second}},
		},
		{
			name: "environment over file",
			args: []string{"--config", path},
			env:  map[string]string{"DB_HOST": "env-host", "RETRY_DELAYS": "10s"},
			want: testConfig{Host: "env-host", Port: 6000, MaxConns: 7, Delays: []time.Duration{10 * time.Second}},
		},
		{
			name: "command line over environment",
			args: []string{"--config", path, "--db.host=cli-host", "--retry.delays=9s", "--db.max_conns=8"},
			env:  map[string]string{"DB_HOST": "env-host", "RETRY_DELAYS": "10s", "DB_MAX_CONNS": "3"},
			want: testConfig{Host: "cli-host", Port: 6000, MaxConns: 8, Delays: []time.Duration{9 * time.Second}},
		},
		{
			name: "environment without file",
			env:  map[string]string{"DB_PORT": "6543", "DB_MAX_CONNS": "2"},
			want: testConfig{Host: "localhost", Port: 6543, MaxConns: 2, Delays: []time.Duration{time.Second}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg testConfig
			require.NoError(t, Load("test", &cfg, tt.args, env(tt.env)))
			assert.Equal(t, tt.want, cfg)
		})
	}
}

func TestLoadUnknownFileKey(t *testing.T) {
	path := writeFile(t, "db:\n  hots: x\nconfig: other.yml\n")

	var cfg testConfig
	err := Load("test", &cfg, []string{"--config", path}, env(nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), path+": unknown key db.hots")
	assert.Contains(t, err.Error(), path+": unknown key config")
}

func TestLoadFileKeyWithoutValue(t *testing.T) {
	path := writeFile(t, "db:\n  host:\n")

	var cfg testConfig
	err := Load("test", &cfg, []string{"--config", path}, env(nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.host has no value")
}

func TestLoadDuplicateFileKeys(t *testing.T) {
	path := writeFile(t, "db:\n  max_conns: 1\n  max-conns: 2\n")

	var cfg testConfig
	err := Load("test", &cfg, []string{"--config", path}, env(nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), path+": db.max-conns and db.max_conns both set db.max-conns")
}

func TestLoadReportsEveryFileAndEnvironmentError(t *testing.T) {
	path := writeFile(t, "db:\n  port:\n  host:\n  hots: x\n  max_conns: 1\n  max-conns: 2\nretry:\n  delays:\n")

	var cfg testConfig
	err := Load("test", &cfg, []string{"--config", path}, env(map[string]string{"DB_PORT": "tcp://x"}))

	require.Error(t, err)
	assert.Equal(t, []string{
		path + ": db.host has no value",
		path + ": db.max-conns and db.max_conns both set db.max-conns",
		path + ": db.port has no value",
		path + ": retry.delays has no value",
		path + ": unknown key db.hots",
		`DB_PORT: db.port: invalid argument "tcp://x" for "--db.port" flag: must be an integer`,
	}, strings.Split(err.Error(), "\n"))
}

func TestLoadSyntaxErrorKeepsEnvironmentErrors(t *testing.T) {
	path := writeFile(t, "db: [\n")

	var cfg testConfig
	err := Load("test", &cfg, []string{"--config", path}, env(map[string]string{"DB_PORT": "tcp://x"}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read config file "+path)
	assert.Contains(t, err.Error(), "DB_PORT: db.port")
}

func TestLoadIgnoresConfigInEnvironment(t *testing.T) {
	path := writeFile(t, "db:\n  host: file-host\n")

	var cfg testConfig
	require.NoError(t, Load("test", &cfg, nil, env(map[string]string{"CONFIG": path})))

	assert.Equal(t, "localhost", cfg.Host)
}

func TestLoadMissingFile(t *testing.T) {
	var cfg testConfig
	err := Load("test", &cfg, []string{"--config", filepath.Join(t.TempDir(), "missing.yml")}, env(nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read config file")
}

func TestLoadEnvironmentCollision(t *testing.T) {
	var cfg collidingConfig
	err := Load("test", &cfg, nil, env(nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "flags db-max.conns and db.max-conns both map to DB_MAX_CONNS")
}

func TestLoadBadValueNamesSource(t *testing.T) {
	path := writeFile(t, "db:\n  max_conns: many\n")

	var cfg testConfig
	err := Load("test", &cfg, []string{"--config", path}, env(map[string]string{
		"DB_PORT":      "tcp://10.0.0.1:5432",
		"RETRY_DELAYS": "soon",
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `DB_PORT: db.port: invalid argument "tcp://10.0.0.1:5432"`)
	assert.Contains(t, err.Error(), `RETRY_DELAYS: retry.delays: invalid argument "soon"`)
	assert.Contains(t, err.Error(), path+`: db.max-conns: invalid argument "many"`)
}

func TestLoadBadCommandLine(t *testing.T) {
	var cfg testConfig
	err := Load("test", &cfg, []string{"--db.port=x"}, env(nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.port")
}

func TestLoadIgnoresUnknownEnvironment(t *testing.T) {
	var cfg testConfig
	require.NoError(t, Load("test", &cfg, nil, env(map[string]string{"PATH": "/bin", "DB": "x"})))
}

func TestEnvName(t *testing.T) {
	assert.Equal(t, "POSTGRES_MAX_CONNS", EnvName("postgres.max-conns"))
	assert.Equal(t, "HTTP_PORT", EnvName("http.port"))
}

func TestFlagNames(t *testing.T) {
	assert.Equal(t, []string{"db.host", "db.max-conns", "db.port", "retry.delays"}, FlagNames(&testConfig{}))
}
