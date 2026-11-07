# Runcoil

Runcoil runs background Tasks for an application and keeps the state of every
Run. This repository is the server, and the home of the protocol, the SDKs and
the dashboard as they land.

**Status: being rebuilt.** The first engine is gone. The server is rebuilt
phase by phase on a new design, and for now it only starts and stops.

## Requirements

- Go 1.27
- [Task](https://taskfile.dev)
- Docker, to build the image

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

Task loads a `.env` file at the root when there is one. Every tool call runs
with `GOWORK=off`, because `go tool -modfile` does not work in workspace mode.

## Layout

- `cmd/runcoil`: the server binary
- `internal/apierr`: the error vocabulary
- `internal/log`: the logger
- `pkg/service`: starts the long-running parts and stops them on a signal
- `tools`: the Go tools, in their own module
- `docker/app`: the Dockerfile of the server

Build the image from the root of the repository:

```sh
docker build -f docker/app/runcoil.dockerfile -t runcoil:local .
```

## License

Apache License 2.0. See [LICENSE](LICENSE).
