# Runcoil Go SDK

The Go SDK runs Runcoil Tasks in your own process. A worker connects to the Runcoil
server, receives Runs, and calls your handlers. Each Step a handler wraps in `step.Do`
is saved on the server, so a Run that starts again skips the Steps already done.

> [!NOTE]
> **The SDK is in early development.** Expect breaking changes until the first release.
> Retries, schedules, `Invoke` and waits are not here yet.

## Install

The SDK needs Go 1.26 or later.

```sh
go get github.com/runcoil/runcoil/sdks/go
```

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

## Configuration

`NewClient` reads its settings from the environment. An option given to `NewClient`
takes precedence.

| Variable | Option | Meaning |
| --- | --- | --- |
| `RUNCOIL_URL` | `runcoil.URL` | The server's worker port, such as `http://localhost:7070`. There is no default |
| `RUNCOIL_API_KEY` | `runcoil.APIKey` | The project's API key (`rc_api_...`) |
| | `runcoil.Logger` | The `*slog.Logger` the SDK writes to. Without it, the SDK logs nothing |

The SDK speaks HTTP/2 only: h2c for an `http` URL, TLS for an `https` URL.

`NewWorker(client, app, opts...)` names the app. Every process that runs the same Tasks
uses the same app name. `runcoil.Slots(n)` sets how many Runs the worker runs at one
time; the default is 10, and the server accepts at most 100.

## Tasks and handlers

- **`runcoil.HandlerFunc(fn)`** adapts `func(context.Context, In) (Out, error)` to a
  handler. The input is decoded from JSON into `In`, and the output is encoded to JSON.
- **The Run fails** when the handler returns an error or panics. An output that JSON
  cannot encode also fails it.
- **`runcoil.RunInfoFromContext(ctx)`** returns the Run ID, app and Task name inside a
  handler.

## Steps

`step.Do(ctx, id, fn)` runs `fn` and saves its result on the server. When the Run runs
again, `step.Do` returns the saved value, or the saved error, and does not call `fn`.

- **Wrap every side effect** in a Step: a payment, an email, a call to another service.
  Code outside a Step runs again each time the Run runs.
- **The Step ID** is `id` the first time it is used in a Run. A later Step with the same
  `id` gets a counter: `charge`, `charge#2`, `charge#3`. A loop that calls one Step is
  fine, as long as it calls the Steps in the same order each time.
- **A failed Step** returns `*step.Error`. The handler may run another Step as a fallback,
  or return the error to fail the Run with that Step's ID. The error is rebuilt from the
  saved text, so `errors.Is` does not match your own error values; use `errors.As` with
  `*step.Error`.
- **Outside a Run**, `step.Do` calls `fn` and returns its result, so handlers can be unit
  tested without a server.

Two rules keep a handler safe when its Run is taken away from the worker, for example
after the worker lost its connection for too long:

- Call `step.Do` on the handler's own goroutine.
- Code that recovers panics must re-panic any value it does not recognise. The SDK stops
  a handler whose Run was taken away by panicking inside `step.Do`.

A Step function should return soon after its context is cancelled.

## Shutdown

`worker.Shutdown(ctx)` stops taking new Runs and waits for the running handlers. When
`ctx` ends first, it cancels the handlers that are still running, waits up to two
seconds, and returns an error naming the Runs it gave up. The server takes those Runs
back later.

## Logs

The SDK logs to the logger given with `runcoil.Logger`, and logs nothing without one.

- At info: registration, shutdown, and the start and end of an outage.
- At debug: each Run and Step, and each reconnect attempt.
- At warn and error: refused reports, panics, and lost connections.

Each line about a Run carries `run_id`, `task_name` and `epoch`.

## Example

[`examples/hello`](examples/hello) is a worker of the app `hello` with one Task,
`greet`, of three Steps. The
[developer guide](../../contribute/developer-guide.md#go-sdk) shows how to run it against
a local server.

## Development

The SDK is its own Go module. Its tasks run with `GOWORK=off`, as CI does.

```sh
task sdk-go:build   # build and vet the SDK and its examples
task sdk-go:test    # tests with the race detector, against an in-process fake server
task sdk-go:check   # lint, test, tidy and vuln
```
