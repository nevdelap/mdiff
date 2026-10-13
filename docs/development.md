# Development

## Prerequisites

- Go 1.26 or newer (the minimum supported version is declared in `go.mod`)
- `just`
- `uv` for Markdown formatting (the tool cache is kept under `/tmp`)
- Docker for Markdown linting
- `yamlfmt` and `yamllint` containers for workflow YAML quality checks
- `golangci-lint` v2.14.0 for Go linting
- Go 1.26.9 for the pinned vulnerability audit

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
just audit
```

The full local gate is:

```sh
just check
```

Use `just run` to launch the terminal prototype. `just test` runs the
simulation, alignment, viewport, and PTY coverage. `just test-race` runs the
complete Go test suite with the race detector. Use `just test-pty` to run only
the standalone terminal interaction tests.

## Go conventions

- `go.mod` records the minimum Go version and module dependencies.
- Format Go source with `gofmt`; do not commit generated binaries or local check
  logs.
- Run `just vet` and `just lint` before handoff. These lint the mdiff packages;
  the namespaced Micro source is pinned upstream code and is verified by its
  package tests and the integration tests rather than by mdiff's lint policy.
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

## Dependency maintenance

Check the module graph for available releases with `go list -m -u all`. Run
`go mod tidy -diff` to verify that `go.mod` and `go.sum` contain no unused
requirements or missing checksums. Update one or more compatible modules, then
run `just check` and `just audit`; inspect both module files and the license
report before handoff. `just audit` installs the pinned govulncheck v1.1.4 and
go-licenses v2.0.1 tools into `/tmp/mdiff-audit-tools`, checks reachable
vulnerabilities and forbidden or unknown dependency licenses, and prints a
license inventory. It uses Go 1.26.9 so the audit itself does not run against
the vulnerable Go 1.26.8 standard library. The Go source minimum remains the
version in `go.mod`.

`github.com/micro-editor/tcell/v2` is intentionally pinned at v2.0.13. The
vendored Micro source calls APIs removed in tcell v2.1.0 and later, including
raw escape registration, paste-mode controls, raw events, and terminal clipboard
methods. The Go module dependency graph and terminal integration tests are the
compatibility gate for that pin. Dependabot ignores tcell v2.1.0 and later until
the vendored Micro adapter is migrated and validated.

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

The CI workflow runs formatting and lint checks, `go test`, PTY checks,
`go vet`, and a build on Linux and macOS with the minimum supported and current
Go release lines. Separate Linux/macOS jobs run the dependency audit and build
the release executable twice on each same runner, requiring byte-identical
outputs and recording the artifact SHA-256 in the workflow summary. The same
commands are available locally through `just`.
