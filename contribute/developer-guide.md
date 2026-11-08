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
task test     # tests with the race detector; needs Docker
task lint     # golangci-lint
task tidy     # fail when go.mod or go.sum is not tidy
task vuln     # govulncheck
task check    # lint, test, tidy and vuln
```

The store tests start PostgreSQL in Docker with testcontainers, capped at 2 vCPUs
and 4 GiB. They use `postgres:18-alpine`; set `POSTGRES_IMAGE` to test another
version, as CI does for 15 and 18. Every tool call runs with `GOWORK=off`,
because `go tool -modfile` does not work in workspace mode.

## Database

The migrations are goose SQL files in `internal/storage/postgres/migration`,
embedded in the `migrate` binary.

```sh
task migrate                     # apply the migrations to the PostgreSQL of Compose
task migrate-create -- <name>    # add a migration
```

## Protocol

The worker protocol is in `proto/`, a Buf module and its own Go module,
`github.com/runcoil/runcoil/proto`. The generated Go code is committed under
`proto/gen/go`.

```sh
task proto:gen      # generate the Go code after changing a .proto file
task proto:format   # format the protos
task proto:check    # lint, format check, generated-code check, build and tidy
```

## Running

```sh
docker compose up -d     # PostgreSQL 18, the migrations, then the server
docker compose down
task run                 # the server on the host, against the PostgreSQL of Compose
```

The server listens on port `7070`:

- `/livez`: the process is alive
- `/readyz`: the database answers, every migration is applied, and the server
  is not shutting down

## Configuration

Each key is a flag (`--postgres.host`), an environment variable with the same
name in upper case (`POSTGRES_HOST`), or a key in a YAML file passed with
`--config`, lowest first: file, environment, flag. `cmd/runcoil/config.example.yml`
and `cmd/runcoil/.env.example` list every key with its default. The binary never
reads `.env`; pass flags to `task run` after `--` to change a key.

## Image

Build the image from the root of the repository:

```sh
docker build -f docker/app/runcoil.dockerfile -t runcoil:local .
docker build -f docker/app/migrate.dockerfile -t runcoil-migrate:local .
```
