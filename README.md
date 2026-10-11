# mdiff

`mdiff` is a standalone Go CLI for a terminal-based, side-by-side diff editor.
It reuses Micro's editing and rendering code and maps both panes onto one
diff-aware viewport so insertions, deletions, and wrapped lines remain aligned.
Each pane has an independent Micro buffer and file identity.

## Run

Requirements: Go 1.26 or newer and [just](https://just.systems/).

```sh
just run
```

The command can also be run directly:

```sh
go run ./cmd/mdiff
```

Pass zero, one, or two file paths to start with empty panes or load files from
left to right:

```sh
mdiff [LEFT [RIGHT]]
just run -- left.txt right.txt
go run ./cmd/mdiff left.txt right.txt
```

`Ctrl+O` opens a file in the focused pane, `Ctrl+S` saves it, and `Ctrl+T`
replaces it with a new unnamed document. Dirty buffers use Micro's
save/discard/cancel prompts. Paste text into either pane, use `Ctrl+W` to switch
panes, `Ctrl+Up`/`Ctrl+Down` or the mouse wheel to scroll both panes together,
and `Alt+Up`/`Alt+Down` to jump between differences. `Tab` keeps its Micro
indentation/completion behavior. Press `Ctrl+Q` to exit. Micro settings and
colorschemes are read from the user's Micro configuration; compatible plugins
are not loaded yet.

Diff updates use an 80 ms debounce by default. Set
`"mdiff": {"diff-debounce-ms": 100}` in Micro's `settings.json` or pass
`--diff-debounce MS` to choose a value from 40 through 180 ms; the command-line
value takes precedence. Large paste events add 20 ms, up to a 200 ms maximum.

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
