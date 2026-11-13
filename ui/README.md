# Dashboard

The web dashboard of Runcoil. It is a single-page app that talks to the dashboard API at `/api/v1`. The server embeds the built files and serves them at every path outside `/api`, so one binary ships both.

## Requirements

- Node 26 (`.nvmrc`)
- pnpm 12.18.0. `package.json` pins it in `devEngines`, and pnpm downloads it when another version runs.

## Develop

Start the server on `:8080`, then install the packages in `ui/` and run the dashboard from the repository root:

```sh
(cd ui && pnpm install)
task ui:dev
```

It serves the dashboard on `http://localhost:5173` and proxies `/api` to the server on `:8080`, so the session cookie works as in production.

## Check

```sh
task ui:check
```

It formats, lints, type checks and runs the tests.

## Generate the API types

```sh
task oapi-gen-client
```

It writes `src/api/schema.d.ts` from the OpenAPI bundle `../openapi/openapi.gen.yaml`. Run it after `task oapi-gen-server` whenever the spec changes. CI fails when the file is stale.

## Build

```sh
task ui:build
```

It builds the dashboard into `dist/`. `embed.go` embeds that folder, so build the dashboard before `go build` to ship it. Without a build, the server answers the dashboard paths with 404. The Docker image builds the dashboard first.

## Layout

- `src/routes/`: the pages, as file routes. `src/routeTree.gen.ts` is generated from them. Do not edit it.
- `src/routes/_app.tsx`: the signed-in layout. It loads the user and sends anyone without a session to `/login`.
- `src/features/<area>/`: the queries, mutations and form schemas of one area, such as `auth`.
- `src/api/`: the API client and the generated types.
- `src/components/`: the app's components. `src/components/ui/` holds the generated base components. Do not edit them by hand.
- `src/lib/`: helpers shared by every area, such as reading API errors into form errors.
- `public/`: files served as they are, such as the favicon.

## Stack

React with TanStack Router and TanStack Query, openapi-fetch for typed API calls, react-hook-form with zod for forms, and Tailwind CSS with Radix-based components. Vite+ builds, lints, formats and tests it.
