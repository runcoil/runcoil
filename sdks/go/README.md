# Runcoil Go SDK

Run durable Tasks in your own Go process. Wrap each side effect in a Step: Runcoil saves
its result, so a crash, a deploy or a retry picks up after the last finished Step instead
of starting over.

> [!NOTE]
> **The SDK is in early development.** Expect breaking changes until the first release.

## Install

```sh
go get github.com/runcoil/runcoil/sdks/go
```

The SDK needs Go 1.26 or later.

## Quick start

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/runcoil/runcoil/sdks/go/pkg/runcoil"
	"github.com/runcoil/runcoil/sdks/go/pkg/step"
)

type orderInput struct {
	OrderID string `json:"order_id"`
}

type orderOutput struct {
	ChargeID string `json:"charge_id"`
}

func processOrder(ctx context.Context, in orderInput) (orderOutput, error) {
	chargeID, err := step.Do(ctx, "charge", func(ctx context.Context) (string, error) {
		return charge(ctx, in.OrderID)
	})
	if err != nil {
		return orderOutput{}, err
	}

	_, err = step.Do(ctx, "email", func(ctx context.Context) (bool, error) {
		return true, sendReceipt(ctx, in.OrderID, chargeID)
	})
	if err != nil {
		return orderOutput{}, err
	}

	return orderOutput{ChargeID: chargeID}, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := runcoil.NewClient() // reads RUNCOIL_URL and RUNCOIL_API_KEY
	if err != nil {
		log.Fatal(err)
	}

	worker, err := runcoil.NewWorker(client, "shop", runcoil.Slots(10))
	if err != nil {
		log.Fatal(err)
	}

	if err := worker.NewTask("process-order", runcoil.HandlerFunc(processOrder)); err != nil {
		log.Fatal(err)
	}

	if err := worker.Start(ctx); err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()

	drainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := worker.Shutdown(drainCtx); err != nil {
		log.Print(err)
	}
}
```

`charge` and `sendReceipt` stand for your own code.

Set `RUNCOIL_URL` to the server's worker port and `RUNCOIL_API_KEY` to your project's API
key, then run it. The [quickstart](https://docs.runcoil.dev/start/quickstart/) starts a
local server for you.

## Retries and schedules

A failed Step retries on its own, with backoff. A Task can also run on cron:

```go
worker.NewTask("process-order", runcoil.HandlerFunc(processOrder),
	runcoil.Retry(retry.Policy{MaxAttempts: 5, Delays: retry.Exponential(time.Second, 2, time.Minute, 4)}),
)

worker.NewTask("cleanup", runcoil.HandlerFunc(cleanup),
	runcoil.Cron("0 3 * * *"), // 03:00 UTC every day
)
```

Inside a Step, `retry.NonRetriable(err)` fails at once, and `retry.After(err, d)` waits
`d` before the next try.

## Learn more

- [Steps](https://docs.runcoil.dev/concepts/steps/): replay, Step IDs, and the rules a
  handler follows
- [Retries](https://docs.runcoil.dev/concepts/retries/): policies, delays, and errors that
  never retry
- [Workers](https://docs.runcoil.dev/concepts/workers/): slots, lost connections,
  shutdown, and logs
- [Tasks and Runs](https://docs.runcoil.dev/concepts/tasks-and-runs/): Run statuses, and
  why a Run fails
- [Go SDK configuration](https://docs.runcoil.dev/reference/go-sdk-configuration/): every
  option and its default

## Development

The SDK is its own Go module. Its tasks run with `GOWORK=off`, as CI does.

```sh
task sdk-go:build   # build and vet the SDK and its examples
task sdk-go:test    # tests with the race detector, against an in-process fake server
task sdk-go:check   # lint, test, tidy and vuln
```

[`examples/hello`](examples/hello) is a small worker; the
[developer guide](../../contribute/developer-guide.md#go-sdk) runs it against a local
server.
