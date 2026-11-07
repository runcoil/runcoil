# Runcoil

Runcoil runs background Tasks for an application and keeps the state of every
Run. This repository is the server, and the home of the protocol, the SDKs and
the dashboard as they land.

**Status: being rebuilt.** The first engine is gone. The server is rebuilt
phase by phase on a new design. For now it loads its config, opens the
PostgreSQL pool, and serves its health checks.

## Requirements

- Go 1.27
- [Task](https://taskfile.dev)
- Docker, for Compose and the image

The Go tools (golangci-lint, govulncheck) are pinned in `tools/go.mod` and run
through `go tool`. Nothing else is installed globally.

## Development

```sh
task build    # build the server
task test     # tests with the race detector
task lint     # golangci-lint
task tidy     # fail when go.mod or go.sum is not tidy
task vuln     # govulncheck
task check    # lint, test, tidy and vuln
```

The tests open no database. Every tool call runs with `GOWORK=off`, because
`go tool -modfile` does not work in workspace mode.

## Running

```sh
task dc-up     # PostgreSQL 18 and the server in Docker Compose
task dc-down
task run       # the server on the host, against the PostgreSQL of task dc-up
```

The server listens on port `7070`:

- `/livez`: the process is alive
- `/readyz`: the database answers and the server is not shutting down

Each key is a flag (`--postgres.host`), an environment variable with the same
name in upper case (`POSTGRES_HOST`), or a key in a YAML file passed with
`--config`, lowest first: file, environment, flag. `cmd/runcoil/config.example.yml`
and `cmd/runcoil/.env.example` list every key with its default. The binary never
reads `.env`; `task run` loads `cmd/runcoil/.env` when it exists.

## Layout

- `cmd/runcoil`: the server binary
- `internal/apierr`: the error vocabulary
- `internal/log`: the logger
- `internal/postgres`: the PostgreSQL pool
- `internal/engine`: the engine; for now the HTTP server and its health checks
- `pkg/configutil`: loads the config from flags, the environment and a file
- `pkg/service`: starts the long-running parts and stops them on a signal
- `tools`: the Go tools, in their own module
- `docker/app`: the Dockerfile of the server

Build the image from the root of the repository:

```sh
docker build -f docker/app/runcoil.dockerfile -t runcoil:local .
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
