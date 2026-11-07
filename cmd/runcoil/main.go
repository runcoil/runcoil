package main

import (
	"context"
	"fmt"
	"os"

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger, err := log.NewLogger(log.Config{Format: log.FormatText})
	if err != nil {
		return fmt.Errorf("new logger: %w", err)
	}

	manager := service.NewManager()

	logger.InfoContext(ctx, "server is started")

	if err := manager.Run(ctx); err != nil {
		return fmt.Errorf("run services: %w", err)
	}

	logger.InfoContext(ctx, "server is stopped")

	return nil
}
