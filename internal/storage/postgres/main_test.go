package postgres

import (
	"cmp"
	"context"
	"crypto/rand"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// testServer is the PostgreSQL that TestMain starts. Each test gets a database of its own on it.
var testServer Config

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests starts PostgreSQL in Docker, capped at 2 vCPUs and 4 GiB, then runs the tests.
// POSTGRES_IMAGE picks the version.
func runTests(m *testing.M) int {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx,
		cmp.Or(os.Getenv("POSTGRES_IMAGE"), "postgres:18-alpine"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.WithDatabase("postgres"),
		tcpostgres.BasicWaitStrategies(),
		testcontainers.WithHostConfigModifier(func(hc *container.HostConfig) {
			hc.NanoCPUs = 2_000_000_000
			hc.Memory = 4 << 30
		}),
	)
	defer func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres: %v\n", err)
		}
	}()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres: %v\n", err)
		return 1
	}

	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres connection string: %v\n", err)
		return 1
	}

	u, err := url.Parse(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse connection string: %v\n", err)
		return 1
	}

	port, err := strconv.Atoi(u.Port())
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse port: %v\n", err)
		return 1
	}

	testServer = Config{
		Host:     u.Hostname(),
		Port:     port,
		User:     "postgres",
		Password: "postgres",
		DB:       "postgres",
		SSLMode:  "disable",
		MaxConns: 4,
	}

	return m.Run()
}

// newTestPool creates an empty database and returns a pool on it.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := t.Context()

	admin, err := NewPgxPool(ctx, testServer)
	require.NoError(t, err)
	defer admin.Close()

	cfg := testServer
	cfg.DB = "test_" + strings.ToLower(rand.Text())

	_, err = admin.Exec(ctx, "CREATE DATABASE "+cfg.DB)
	require.NoError(t, err)

	pool, err := NewPgxPool(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}
