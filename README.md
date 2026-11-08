<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/logo/runcoil-lockup-on-dark.svg">
  <img alt="Runcoil" src="docs/logo/runcoil-lockup-on-light.svg" width="360">
</picture>

Durable background Tasks for your application

[![License](https://img.shields.io/github/license/runcoil/runcoil)](LICENSE)

Runcoil runs your functions durably. A crash, a deploy, or a long wait does not
lose the work, because each finished Step is saved and the function replays past it.

- **Steps:** wrap each piece of work in a Step; a retry or a restart skips the Steps already done.
- **Retries:** a failed Step retries on its own policy, with backoff, before the Run fails.
- **Schedules:** run a Task on cron, at fixed times, or every interval.
- **Waits:** sleep for days, or wait for an Event, without holding a worker.
- **Self-hosted:** one server and PostgreSQL, nothing else to run.

> [!NOTE]
> **Runcoil is in early development.** Expect breaking changes until the first release.

## Get started

```sh
docker compose up -d
curl localhost:8080/readyz
```

## Contributing

Set up your local environment with the [developer guide](contribute/developer-guide.md).

## License

Runcoil is distributed under the [Apache License 2.0](LICENSE).
