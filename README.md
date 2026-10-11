# mdiff

`mdiff` is a standalone Go CLI prototype for a terminal-based, side-by-side diff
editor. It reuses Micro's in-memory editing and rendering code and maps both
panes onto one diff-aware viewport so insertions, deletions, and wrapped lines
remain aligned. The current prototype keeps both buffers in memory and does not
open or save files.

## Run

Requirements: Go 1.26 or newer and [just](https://just.systems/).

```sh
just run
```

The command can also be run directly:

```sh
go run ./cmd/mdiff
```

Run without file arguments. Paste text into either pane, use `F6` to switch
panes, `Ctrl-Up`/`Ctrl-Down` or the mouse wheel to scroll both panes together,
and `Alt-Up`/`Alt-Down` to jump between differences. `Tab` keeps its Micro
indentation/completion behavior. Press `Ctrl-Q` to exit. Micro settings and
colorschemes are read from the user's Micro configuration; plugins are not
loaded.

## Development

See [docs/development.md](docs/development.md) for setup, formatting, linting,
testing, build, and CI-equivalent commands.

Contributor responsibilities and the independent review workflow are described
in [docs/roles.md](docs/roles.md) and
[`design_docs/agent_workflow.md`](design_docs/agent_workflow.md).

The technical design, prototype findings, and implementation milestones are in
[design_docs/micro_diff_editor.md](design_docs/micro_diff_editor.md),
[design_docs/micro_integration_prototype.md](design_docs/micro_integration_prototype.md),
and [design_docs/implementation_plan.md](design_docs/implementation_plan.md).

## License

No project license has been selected yet. Micro's upstream and third-party
notices are preserved in `internal/micro/`.
