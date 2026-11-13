# Dashboard API spec

The OpenAPI 3.1 document of the dashboard API. The server code in `internal/api/gen` is generated from it.

## Layout

- `openapi.yaml`: info, servers, security, and every path by `$ref`. It lists no components.
- `paths/<resource>.yaml`: one file per resource, one key per path, such as `paths/auth.yaml#/login`.
- `components/schemas/<resource>.yaml`, `components/schemas/errors.yaml`, `components/responses.yaml`.
- `openapi.gen.yaml`: the bundle, committed. Do not edit it.

## Rules

- Every operation has an `operationId` in lowerCamelCase, a summary, a description and a tag.
- Every operation declares its permission in `x-runcoil-permission`, as `resource:verb`, or `self` for the caller's own account. Only `login` has `security: []` and no permission.
- Every operation lists all its responses. Shared errors come from `components/responses.yaml`.
- Every object has `required` and `additionalProperties: false`. Every property has a description, an example and an `x-order`.
- A path key is a valid component name: letters, digits, `.`, `_` and `-`.

## Generate

```sh
task api:gen
```

It bundles the spec, lints the bundle with vacuum, and generates the server with oapi-codegen.
