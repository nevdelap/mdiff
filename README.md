# mdiff

`mdiff` is a planned standalone, terminal-based side-by-side diff editor written
in Go. The design reuses Micro editor capabilities and defines a shared
diff-aware viewport so insertions, deletions, and wrapped lines remain aligned.

The application is currently a Go project scaffold. Its command prints a
hello-world message; the editor has not been implemented.

## Run

Requirements: Go 1.26 or newer and [just](https://just.systems/).

```sh
just run
```

The command can also be run directly:

```sh
go run ./cmd/mdiff
```

## Development

See [docs/development.md](docs/development.md) for setup, formatting, linting,
testing, build, and CI-equivalent commands.

Contributor responsibilities and the independent review workflow are described
in [docs/roles.md](docs/roles.md) and
[`design_docs/agent_workflow.md`](design_docs/agent_workflow.md).

The technical design and implementation milestones are in
[design_docs/micro_diff_editor.md](design_docs/micro_diff_editor.md).

## License

No project license has been selected yet. The Micro integration plan includes
preserving upstream license and third-party notices when implementation begins.
