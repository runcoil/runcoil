package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"maps"
	"math"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/runcoil/runcoil/sdks/go/pkg/runcoil"
	"github.com/runcoil/runcoil/sdks/go/pkg/step"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var level slog.Level
	if err := level.UnmarshalText([]byte(envOr("LOG_LEVEL", "info"))); err != nil {
		return fmt.Errorf("parse LOG_LEVEL: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	client, err := runcoil.NewClient(runcoil.Logger(logger))
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}

	worker, err := runcoil.NewWorker(client, "playground", runcoil.Slots(4))
	if err != nil {
		return fmt.Errorf("new worker: %w", err)
	}

	tasks := map[string]runcoil.Handler{
		"order":      runcoil.HandlerFunc(order),
		"fallback":   runcoil.HandlerFunc(fallback),
		"loop":       runcoil.HandlerFunc(loop),
		"nested":     runcoil.HandlerFunc(nested),
		"panic":      runcoil.HandlerFunc(panics),
		"bad_output": runcoil.HandlerFunc(badOutput),
		"error":      runcoil.HandlerFunc(plainError),
		"slow":       runcoil.HandlerFunc(slow),
		"ignore_ctx": runcoil.HandlerFunc(ignoreCtx),
		"big":        runcoil.HandlerFunc(big),
	}

	for _, name := range slices.Sorted(maps.Keys(tasks)) {
		if err := worker.NewTask(name, tasks[name]); err != nil {
			return fmt.Errorf("new task %s: %w", name, err)
		}
	}

	if err := worker.Start(ctx); err != nil {
		return fmt.Errorf("start worker: %w", err)
	}

	<-ctx.Done()

	drainCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := worker.Shutdown(drainCtx); err != nil {
		return fmt.Errorf("shutdown worker: %w", err)
	}

	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

type orderInput struct {
	OrderID string  `json:"order_id"`
	Amount  float64 `json:"amount"`
	// FailAt names the Step that fails: reserve, charge or ship.
	FailAt string `json:"fail_at"`
	// Pause makes each Step take this long, such as "3s".
	Pause string `json:"pause"`
}

type orderOutput struct {
	OrderID  string `json:"order_id"`
	ChargeID string `json:"charge_id"`
	Tracking string `json:"tracking"`
}

// order runs three Steps in a row. A failed Step fails the Run with its step_id.
func order(ctx context.Context, in orderInput) (orderOutput, error) {
	stepFn := func(name string, value func() string) func(context.Context) (string, error) {
		return func(ctx context.Context) (string, error) {
			if err := pause(ctx, in.Pause); err != nil {
				return "", err
			}

			if in.FailAt == name {
				return "", fmt.Errorf("%s failed for order %s", name, in.OrderID)
			}

			return value(), nil
		}
	}

	if _, err := step.Do(ctx, "reserve", stepFn("reserve", func() string { return "reserved" })); err != nil {
		return orderOutput{}, fmt.Errorf("reserve: %w", err)
	}

	chargeID, err := step.Do(ctx, "charge", stepFn("charge", func() string {
		return fmt.Sprintf("ch_%s_%d", in.OrderID, time.Now().UnixNano())
	}))
	if err != nil {
		return orderOutput{}, fmt.Errorf("charge: %w", err)
	}

	tracking, err := step.Do(ctx, "ship", stepFn("ship", func() string { return "trk_" + strings.ToUpper(in.OrderID) }))
	if err != nil {
		return orderOutput{}, fmt.Errorf("ship: %w", err)
	}

	return orderOutput{OrderID: in.OrderID, ChargeID: chargeID, Tracking: tracking}, nil
}

// fallback runs a second Step when the first one fails, so the Run succeeds with a failed
// Step in its journal.
func fallback(ctx context.Context, _ struct{}) (string, error) {
	got, err := step.Do(ctx, "primary", func(context.Context) (string, error) {
		return "", errors.New("primary is down")
	})

	var stepErr *step.Error
	if errors.As(err, &stepErr) {
		return step.Do(ctx, "secondary", func(context.Context) (string, error) {
			return "served by secondary after: " + stepErr.Message, nil
		})
	}

	return got, err
}

type loopInput struct {
	Count int `json:"count"`
}

// loop calls a Step with the same id count times, so the ids get counters: chunk,
// chunk#2, chunk#3.
func loop(ctx context.Context, in loopInput) (int, error) {
	sum := 0
	for i := range in.Count {
		n, err := step.Do(ctx, "chunk", func(context.Context) (int, error) { return i * i, nil })
		if err != nil {
			return 0, fmt.Errorf("chunk %d: %w", i, err)
		}

		sum += n
	}

	return sum, nil
}

// nested calls a Step inside another Step.
func nested(ctx context.Context, _ struct{}) (string, error) {
	return step.Do(ctx, "outer", func(ctx context.Context) (string, error) {
		inner, err := step.Do(ctx, "inner", func(context.Context) (string, error) { return "inner value", nil })
		if err != nil {
			return "", fmt.Errorf("inner: %w", err)
		}

		return "outer wraps " + inner, nil
	})
}

type whereInput struct {
	// Where is "handler" or "step".
	Where string `json:"where"`
}

// panics panics in the handler or in a Step.
func panics(ctx context.Context, in whereInput) (string, error) {
	if in.Where == "step" {
		return step.Do(ctx, "explode", func(context.Context) (string, error) { panic("boom in a step") })
	}

	panic("boom in the handler")
}

// badOutput returns an output that JSON cannot encode, from the handler or from a Step.
func badOutput(ctx context.Context, in whereInput) (float64, error) {
	if in.Where == "step" {
		return step.Do(ctx, "nan", func(context.Context) (float64, error) { return math.NaN(), nil })
	}

	return math.Inf(1), nil
}

// plainError fails the Run with an error that no Step returned.
func plainError(context.Context, struct{}) (struct{}, error) {
	return struct{}{}, errors.New("plain handler error")
}

type pauseInput struct {
	Pause string `json:"pause"`
}

// slow runs one Step that waits, and returns when its context ends.
func slow(ctx context.Context, in pauseInput) (string, error) {
	return step.Do(ctx, "wait", func(ctx context.Context) (string, error) {
		if err := pause(ctx, in.Pause); err != nil {
			return "", err
		}

		return "waited " + in.Pause, nil
	})
}

// ignoreCtx runs one Step that sleeps and ignores its context.
func ignoreCtx(ctx context.Context, in pauseInput) (string, error) {
	return step.Do(ctx, "sleep", func(context.Context) (string, error) {
		d, err := time.ParseDuration(in.Pause)
		if err != nil {
			return "", fmt.Errorf("parse pause: %w", err)
		}

		time.Sleep(d)

		return "slept " + in.Pause, nil
	})
}

type bigInput struct {
	// Bytes is the size of the Step output.
	Bytes int `json:"bytes"`
}

// big returns a Step output of the given size.
func big(ctx context.Context, in bigInput) (int, error) {
	s, err := step.Do(ctx, "blob", func(context.Context) (string, error) { return strings.Repeat("x", in.Bytes), nil })
	if err != nil {
		return 0, fmt.Errorf("blob: %w", err)
	}

	return len(s), nil
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
		return fmt.Errorf("pause: %w", ctx.Err())
	}
}
