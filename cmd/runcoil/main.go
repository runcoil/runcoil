package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/runcoil/runcoil/internal/log"
	"github.com/runcoil/runcoil/pkg/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("error running application: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger, err := log.NewLogger(log.Config{Format: log.FormatText})
	if err != nil {
		return fmt.Errorf("new logger: %w", err)
	}

	manager := service.NewManager()

	logger.InfoContext(ctx, "server is started")

	if err := manager.Run(ctx); err != nil {
		return fmt.Errorf("run services: %w", err)
	}

	// The manager returns at once while it has no services, so wait for the signal here.
	<-ctx.Done()

	logger.InfoContext(ctx, "server is stopped")

	return nil
}
