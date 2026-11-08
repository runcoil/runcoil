package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migration/*.sql
var migrationFS embed.FS

// Migrator applies the embedded migrations. The server only checks them; cmd/migrate applies them.
type Migrator struct {
	provider *goose.Provider
}

// NewMigrator creates a Migrator for the database of pool.
func NewMigrator(pool *pgxpool.Pool) (*Migrator, error) {
	migrations, err := fs.Sub(migrationFS, "migration")
	if err != nil {
		return nil, fmt.Errorf("open migrations: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(pool), migrations)
	if err != nil {
		return nil, fmt.Errorf("new goose provider: %w", err)
	}

	return &Migrator{provider: provider}, nil
}

// Up applies every pending migration.
func (m *Migrator) Up(ctx context.Context) error {
	if _, err := m.provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}

	return nil
}

// HasPending reports whether a migration is not applied yet.
func (m *Migrator) HasPending(ctx context.Context) (bool, error) {
	pending, err := m.provider.HasPending(ctx)
	if err != nil {
		return false, fmt.Errorf("check pending migrations: %w", err)
	}

	return pending, nil
}
