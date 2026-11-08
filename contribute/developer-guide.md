# Developer guide

## Requirements

- Go 1.27
- [Task](https://taskfile.dev)
- Docker, for Compose and the image

The Go tools (golangci-lint, govulncheck, goose, buf) are pinned in `tools/go.mod` and run
through `go tool`. Nothing else is installed globally.

## Development

```sh
task setup    # download the modules and build the Go tools
task build    # build the server
task test     # tests with the race detector
task lint     # golangci-lint
task tidy     # fail when go.mod or go.sum is not tidy
task vuln     # govulncheck
task check    # lint, test, tidy and vuln
```

The tests open no database. Every tool call runs with `GOWORK=off`, because
`go tool -modfile` does not work in workspace mode.

## Database

The migrations are goose SQL files in `internal/storage/postgres/migration`,
embedded in the server and in `runcoil-migrate`, a binary that applies them and
exits. With `auto-migrate` set, the server applies them at boot; Compose
sets it. The tasks run goose against the PostgreSQL
of Compose, or against `GOOSE_DBSTRING` when it is set.

The queries are sqlc files in `internal/storage/postgres/sqlc`, one per table;
the generated Go code is committed next to them.

```sh
task sqlc-gen                    # generate the Go code after changing a query or a migration
task migrate-up                  # apply every pending migration
task migrate-down                # roll back the last one
task migrate-reset               # roll back every migration
task migrate-status              # show which are applied
task migrate-create -- <name>    # add a migration
```

## Protocol

The worker protocol is in `proto/`, a Buf module and its own Go module,
`github.com/runcoil/runcoil/proto`. The generated Go code is committed under
`proto/gen/go`.

Each merge to `main` that changes `proto/` tags a new patch version,
`proto/vX.Y.Z`, through `proto-release.yaml`; `#minor` or `#major` in the commit
message bumps that part instead. The Go SDK and other users import a tag.

```sh
task proto:gen      # generate the Go code after changing a .proto file
task proto:format   # format the protos
task proto:check    # lint, format check, generated-code check, build and tidy
```

## Running

```sh
docker compose up -d     # PostgreSQL 18 and the server, which migrates at boot
docker compose down
task run                 # the server on the host, against the PostgreSQL of Compose
```

The server listens on port `7070`:

- `/livez`: the process is alive
- `/readyz`: the database answers and the server is not shutting down

The same port serves the worker protocol, `runcoil.v1.WorkerService`.

## A Run by hand

Until the SDK and the client API exist, a Run can be followed by playing the worker
with `task worker-call`, which sends one call with `buf curl`. Compose adds the API
key `rc_api_local_dev` to the project `default` at boot (`BOOTSTRAP_API_KEY`).
`bytes` fields such as `input` and `output` are base64 in JSON.

1. Start the server and register a worker. Keep the `sessionToken` it returns:

   ```sh
   docker compose up -d --build
   task worker-call METHOD=Register TOKEN=rc_api_local_dev \
     DATA='{"app":"shop","slots":2,"tasks":[{"name":"charge"}]}'
   ```

2. Open the stream in a second terminal. It prints an `assign` for each Run the
   server gives the worker, and a `keepalive` every heartbeat interval:

   ```sh
   task worker-call METHOD=Listen TOKEN=<session token>
   ```

3. Create a Run of the Task. It is assigned within `engine.poll-interval`:

   ```sh
   task dev-run-create APP=shop TASK=charge INPUT='{"orderId":42}'
   ```

4. Record a Step and end the Run with the `lease` of the `assign`:

   ```sh
   task worker-call METHOD=CompleteStep TOKEN=<session token> \
     DATA='{"lease":{"runId":"<run id>","epoch":"1"},"stepId":"charge","output":"eyJvayI6dHJ1ZX0="}'
   task worker-call METHOD=CompleteRun TOKEN=<session token> \
     DATA='{"lease":{"runId":"<run id>","epoch":"1"},"output":"eyJvayI6dHJ1ZX0="}'
   task worker-call METHOD=Heartbeat TOKEN=<session token>
   ```

5. Look at the tables:

   ```sh
   docker compose exec postgres psql -U postgres \
     -c 'select id, status, error_code from runs' \
     -c 'select run_id, step_id, status from steps' \
     -c 'select run_id, lease_epoch from run_leases'
   ```

A Step error is `"error":{"kind":"ERROR_KIND_ERROR","message":"..."}`, a Run error the
same with `"stepId"` when it is a Step's final error. A call with an old `epoch` fails
with `STALE_LEASE`. Stopping Listen and opening it again with
`DATA='{"running":[<lease>]}'` reconciles: a lease the worker does not report goes
back to the queue, and a reported one it no longer holds gets a `cancel`.

## Configuration

Each key is a flag (`--postgres.host`), an environment variable with the same
name in upper case (`POSTGRES_HOST`), or a key in a YAML file passed with
`--config`, lowest first: file, environment, flag. Each binary has a
`config.example.yml` and an `.env.example` in its `cmd/` directory that list every
key with its default. No binary reads `.env`; pass flags to `task run` after `--`
to change a key.

## Image

Build the image from the root of the repository:

```sh
docker build -f docker/app/runcoil.dockerfile -t runcoil:local .
```
