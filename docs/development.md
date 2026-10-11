# Development

## Prerequisites

- Go 1.26 or newer (the minimum supported version is declared in `go.mod`)
- `just`
- `uv` for Markdown formatting (the tool cache is kept under `/tmp`)
- Docker for Markdown linting
- `yamlfmt` and `yamllint` containers for workflow YAML quality checks
- `golangci-lint` v2.14.0 for Go linting

Install Go dependencies with:

```sh
just install
```

Install the pinned Go linter with:

```sh
just install-tools
```

The install recipes do not install system packages or change your Go toolchain.
If `golangci-lint` is not on your `PATH`, add `$(go env GOPATH)/bin`.

## Local loop

List available recipes with `just`. Format all Go, Markdown, and Just files
with:

```sh
just format
```

Run formatting checks, Go lint, and Markdown lint with:

```sh
just lint
```

Run the standard Go tests and build with:

```sh
just test
just build
```

The full local gate is:

```sh
just check
```

Use `just run` to execute the scaffold command. `just test-race` runs Go tests
with the race detector. The current scaffold has no test files yet, but the test
recipes are ready for implementation work.

## Go conventions

- `go.mod` records the minimum Go version and module dependencies.
- Format Go source with `gofmt`; do not commit generated binaries or local check
  logs.
- Run `go vet ./...` and `golangci-lint run` before handoff.
- Keep tests beside the package they exercise and run them with the Go standard
  test runner.
- Use `go mod tidy` when dependency declarations change, then inspect both
  `go.mod` and `go.sum`.
- Keep `just` as the documented command interface for common development work.

## Documentation and quality

Markdown uses `mdformat` with an 80-column wrap and Markdownlint settings in
`.mdformat.toml` and `.markdownlint.yaml`. Workflow YAML uses `yamlfmt` and
`yamllint`; `just --fmt` formats the Justfile. CI runs Go tests, vet, and builds
on Linux and macOS. Adjust CI timeouts using observed job runtime, with room for
cache misses and dependency downloads.

`check.log` contains output from quiet recipes such as `just qcheck`. Read it
when a quiet check fails.

## Repository workflow

This checkout uses Jujutsu. Use `jj` for repository status, history, and diffs
when the working-copy metadata is writable. Do not add Git-specific manual
development steps where Jujutsu provides the same operation. `difft` is a useful
optional patch viewer. The project can be built and checked without either
Jujutsu or difftastic.

`uv` is used only to run the pinned Markdown formatter. It does not introduce a
Python application environment.

Contributor role expectations and the task handoff, review, and state workflow
are documented in [roles.md](roles.md) and
[`design_docs/agent_workflow.md`](../design_docs/agent_workflow.md).

## Continuous integration

The CI workflow runs formatting and lint checks, `go test`, `go vet`, and a
build on Linux and macOS with the minimum supported and current Go release
lines. The same commands are available locally through `just`.
