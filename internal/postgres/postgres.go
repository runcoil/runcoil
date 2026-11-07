package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/pflag"
)

const connectTimeout = 10 * time.Second

var sslModes = []string{"disable", "prefer", "require", "verify-ca", "verify-full"}

// Config is the PostgreSQL connection, set by its parts.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DB       string
	SSLMode  string
	MaxConns int32
}

// RegisterFlags declares every key with its default and help text.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Host, "postgres.host", "localhost", "PostgreSQL host.")
	fs.IntVar(&c.Port, "postgres.port", 5432, "PostgreSQL port.")
	fs.StringVar(&c.User, "postgres.user", "runcoil", "PostgreSQL user.")
	fs.StringVar(&c.Password, "postgres.password", "", "PostgreSQL password.")
	fs.StringVar(&c.DB, "postgres.db", "runcoil", "PostgreSQL database.")
	fs.StringVar(&c.SSLMode, "postgres.ssl-mode", "prefer", "TLS mode, as libpq names it: "+strings.Join(sslModes, ", ")+".")
	fs.Int32Var(&c.MaxConns, "postgres.max-conns", 10, "Most connections in the pool.")
}

// Validate returns every invalid value at once.
func (c *Config) Validate() error {
	var errs []error

	if c.Host == "" {
		errs = append(errs, errors.New("postgres.host is required"))
	}
	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, errors.New("postgres.port must be between 1 and 65535"))
	}
	if c.User == "" {
		errs = append(errs, errors.New("postgres.user is required"))
	}
	if c.DB == "" {
		errs = append(errs, errors.New("postgres.db is required"))
	}
	if !slices.Contains(sslModes, c.SSLMode) {
		errs = append(errs, fmt.Errorf("postgres.ssl-mode must be one of %s", strings.Join(sslModes, ", ")))
	}
	if c.MaxConns < 1 {
		errs = append(errs, errors.New("postgres.max-conns must be at least 1"))
	}

	return errors.Join(errs...)
}

// connString builds a libpq keyword/value string, with each value quoted. The
// password stays out: pgx redacts it from errors only up to the first quote.
func (c *Config) connString() string {
	parts := []string{
		"host=" + quote(c.Host),
		"port=" + strconv.Itoa(c.Port),
		"user=" + quote(c.User),
		"dbname=" + quote(c.DB),
		"sslmode=" + quote(c.SSLMode),
	}

	return strings.Join(parts, " ")
}

func quote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// NewPool opens the pool and checks that the database answers.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.connString())
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Password != "" {
		poolCfg.ConnConfig.Password = cfg.Password
	}
	poolCfg.MaxConns = cfg.MaxConns

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}
