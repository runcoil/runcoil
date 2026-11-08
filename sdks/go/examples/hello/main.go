// Command hello runs a worker of the app "hello" with one Task, "greet", of three Steps.
// Start a server, then run it with RUNCOIL_URL and RUNCOIL_API_KEY set. Stop it with
// Ctrl-C; it waits up to 30 seconds for running handlers.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/runcoil/runcoil/sdks/go/pkg/runcoil"
	"github.com/runcoil/runcoil/sdks/go/pkg/step"
)

type greetInput struct {
	Name string `json:"name"`
	// FailAt names a Step that fails, to see a failed Run.
	FailAt string `json:"fail_at"`
	// Pause makes the compose Step take this long, such as "5s", to see a Run in flight.
	Pause string `json:"pause"`
}

type greetOutput struct {
	Greeting string `json:"greeting"`
}

type tasks struct {
	logger *slog.Logger
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client, err := runcoil.NewClient(runcoil.Logger(logger))
	if err != nil {
		return err
	}

	worker, err := runcoil.NewWorker(client, runcoil.App("hello"), runcoil.Slots(2))
	if err != nil {
		return err
	}

	t := &tasks{logger: logger}
	if err := worker.NewTask("greet", runcoil.HandlerFunc(t.greet)); err != nil {
		return err
	}

	if err := worker.Start(ctx); err != nil {
		return err
	}

	<-ctx.Done()

	drainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return worker.Shutdown(drainCtx)
}

func (t *tasks) greet(ctx context.Context, in greetInput) (greetOutput, error) {
	name, err := step.Do(ctx, "normalize", func(context.Context) (string, error) {
		if in.FailAt == "normalize" {
			return "", errors.New("normalize failed")
		}

		return strings.TrimSpace(in.Name), nil
	})
	if err != nil {
		return greetOutput{}, err
	}

	greeting, err := step.Do(ctx, "compose", func(ctx context.Context) (string, error) {
		if in.FailAt == "compose" {
			return "", errors.New("compose failed")
		}

		if err := pause(ctx, in.Pause); err != nil {
			return "", err
		}

		return fmt.Sprintf("Hello, %s!", name), nil
	})
	if err != nil {
		return greetOutput{}, err
	}

	_, err = step.Do(ctx, "log", func(ctx context.Context) (bool, error) {
		if in.FailAt == "log" {
			return false, errors.New("log failed")
		}

		info, _ := runcoil.Info(ctx)
		t.logger.InfoContext(ctx, "greeted",
			slog.String("run_id", info.RunID),
			slog.String("greeting", greeting),
		)

		return true, nil
	})
	if err != nil {
		return greetOutput{}, err
	}

	return greetOutput{Greeting: greeting}, nil
}

// pause waits for d, a duration such as "5s", or until ctx ends. An empty d does not wait.
func pause(ctx context.Context, d string) error {
	if d == "" {
		return nil
	}

	wait, err := time.ParseDuration(d)
	if err != nil {
		return fmt.Errorf("parse pause: %w", err)
	}

	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
