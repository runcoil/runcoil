package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/runcoil/runcoil/internal/http"
	"github.com/runcoil/runcoil/internal/log"
	"github.com/runcoil/runcoil/internal/storage/postgres"
	"github.com/runcoil/runcoil/pkg/configutil"
	"github.com/runcoil/runcoil/pkg/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error running application: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cfg Config
	loadErr := configutil.Load("runcoil", &cfg, os.Args[1:], os.LookupEnv)
	if errors.Is(loadErr, pflag.ErrHelp) {
		return nil
	}

	if err := errors.Join(loadErr, cfg.Validate()); err != nil {
		return fmt.Errorf("invalid config:\n%w", err)
	}

	logger := log.NewLogger(cfg.Log)

	pool, err := postgres.NewPgxPool(ctx, cfg.Postgres)
	if err != nil {
		return fmt.Errorf("new pgx pool: %w", err)
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		logger.InfoContext(ctx, "running migrations")

		if err := postgres.Migrate(pool); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	manager := service.NewManager(
		http.NewService(cfg.HTTP, logger, pool),
	)

	logger.InfoContext(ctx, "server is started")

	if err := manager.Run(ctx); err != nil {
		return fmt.Errorf("run services: %w", err)
	}

	logger.InfoContext(ctx, "server is stopped")

	return nil
}
