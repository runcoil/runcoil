package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/runcoil/runcoil/pkg/cmdutil"
)

func validConfig() Config {
	return Config{
		Host:     "localhost",
		Port:     5432,
		User:     "runcoil",
		DB:       "runcoil",
		SSLMode:  "prefer",
		MaxConns: 10,
	}
}

func TestConfigDefaults(t *testing.T) {
	var cfg Config
	require.NoError(t, cmdutil.Load("test", &cfg, nil, func(string) (string, bool) { return "", false }))

	assert.Equal(t, validConfig(), cfg)
	assert.NoError(t, cfg.Validate())
}

func TestValidate(t *testing.T) {
	cfg := Config{Port: 70000, SSLMode: "sometimes"}
	err := cfg.Validate()

	require.Error(t, err)
	for _, want := range []string{
		"postgres.host is required",
		"postgres.port must be between 1 and 65535",
		"postgres.user is required",
		"postgres.db is required",
		"postgres.ssl-mode must be one of disable, prefer, require, verify-ca, verify-full",
		"postgres.max-conns must be at least 1",
	} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestConnStringEscapes(t *testing.T) {
	cfg := validConfig()
	cfg.User = `it's a \ user=x`
	cfg.DB = "my db"
	cfg.SSLMode = "disable"

	parsed, err := pgconn.ParseConfig(cfg.connString())

	require.NoError(t, err)
	assert.Equal(t, "localhost", parsed.Host)
	assert.Equal(t, uint16(5432), parsed.Port)
	assert.Equal(t, `it's a \ user=x`, parsed.User)
	assert.Equal(t, "my db", parsed.Database)
	assert.Nil(t, parsed.TLSConfig)
}

func TestPasswordNeverInErrors(t *testing.T) {
	cfg := validConfig()
	cfg.Host = "127.0.0.1"
	cfg.Port = 1
	cfg.Password = `zebra\'quartz violet-7731`

	connString := cfg.connString()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, connectErr := NewPool(ctx, cfg)
	require.ErrorContains(t, connectErr, "ping")

	badSSLMode := cfg
	badSSLMode.SSLMode = `prefer' sslmode='nope`
	_, parseErr := NewPool(ctx, badSSLMode)
	require.ErrorContains(t, parseErr, "parse config")

	for _, part := range []string{"zebra", "quartz", "violet", "7731"} {
		assert.NotContains(t, connString, part)
		assert.NotContains(t, connectErr.Error(), part)
		assert.NotContains(t, parseErr.Error(), part)
	}
}
