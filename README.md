# mdiff

`mdiff` is a standalone Go CLI for editing and comparing two files in
side-by-side Micro panes. Each pane owns an independent Micro buffer and file
identity. A shared diff-aware visual viewport aligns inserted and deleted lines,
character-level changes, and wrapped rows without adding spacer text to either
buffer.

## Run

Requirements: Go 1.26 or newer and [just](https://just.systems/) for the
development commands.

```sh
just run
```

The command can also be run directly:

```sh
go run ./cmd/mdiff
```

Run with no paths for two empty, editable panes. Pass up to two paths to load
files into the left and right panes. One path leaves the right pane empty.

```sh
mdiff [--config-dir DIR] [--diff-debounce MS] [LEFT [RIGHT]]
just run -- left.txt right.txt
go run ./cmd/mdiff left.txt right.txt
```

`Ctrl+W` switches focus between panes. Edit with Micro's normal movement,
selection, indentation, undo/redo, search, and configured buffer actions. Paste
using the terminal's normal paste shortcut. `Ctrl+Up`/`Ctrl+Down` and the mouse
wheel scroll both panes together; `Alt+Up`/`Alt+Down` jump to the previous or
next difference. Wrapping is controlled by the Micro settings. The visible
cursor follows the focused pane and the shared viewport keeps both panes aligned
during edits, scrolling, and resize.

`Ctrl+O` opens a file in the focused pane, `Ctrl+S` saves that pane, and
`Ctrl+T` replaces it with a new unnamed document. Dirty buffers use Micro's
save/discard/cancel prompts, with the pane identified in the prompt. `Ctrl+Q`
quits. The footer shows the supported Micro-style key hints and pane-switch
binding. Initial file arguments, open, and save are the supported file
workflows; there is no Save As command. Explicit saves write the selected
document.

Run `mdiff --help` for flags and `mdiff --version` for the build version.
Invalid flags, more than two paths, and an invalid debounce value are rejected
before raw terminal mode is entered.

Diff updates use an 80 ms debounce by default. Set
`"mdiff": {"diff-debounce-ms": 100}` in Micro's `settings.json` or pass
`--diff-debounce MS` to choose a value from 40 through 180 ms; the command-line
value takes precedence. Large paste events add 20 ms, up to a 200 ms maximum.

## Micro configuration

mdiff reads Micro's config directory in this order: `--config-dir`,
`MICRO_CONFIG_HOME`, `$XDG_CONFIG_HOME/micro`, then `~/.config/micro`. It reads
`settings.json`, `bindings.json`, user colorschemes, and syntax definitions
without creating config files or directories. Supported Micro buffer settings
apply to both panes, including indentation, wrapping, ruler options, colors, and
configured filetype. mdiff-owned pane switching, quitting, help, and diff
navigation keys remain reserved. Unsupported keybinding actions are reported in
the status area. Micro's private-view `ScrollUp`, `ScrollDown`, `Center`,
`Start`, `End`, `PageUp`, `PageDown`, `HalfPageUp`, and `HalfPageDown` actions
are also rejected because mdiff has one shared diff-aware viewport.

Lua plugins load from Micro's normal `init.lua` and `plug/` locations, and
bundled Micro plugins run by default. Plugins can edit the focused pane and use
Micro's Lua APIs. Operations that create extra tabs/panes or independent
vertical views are reported as unavailable. Plugins are trusted Lua code and
retain Micro's file, process, and network access; mdiff does not sandbox them.

Malformed settings stop startup before raw terminal mode. The selected
colorscheme is resolved after Micro plugins initialize so a plugin can provide
it; an unavailable or malformed scheme aborts startup and restores the terminal.
Invalid bindings or syntax are reported without rewriting the user's files.
Explicit document saves and configured Micro backup/autosave options can write
document artifacts. See the observed support matrix in
[`design_docs/micro_diff_editor.md`](design_docs/micro_diff_editor.md).

## Development

See [docs/development.md](docs/development.md) for setup, formatting, linting,
testing, build, audit, and CI-equivalent commands. See
[docs/release_notes.md](docs/release_notes.md) for known compatibility and
platform limitations.

Contributor responsibilities and the independent review workflow are described
in [docs/roles.md](docs/roles.md) and
[`design_docs/agent_workflow.md`](design_docs/agent_workflow.md).

The technical design, prototype findings, and implementation milestones are in
[design_docs/micro_diff_editor.md](design_docs/micro_diff_editor.md),
[design_docs/micro_integration_prototype.md](design_docs/micro_integration_prototype.md),
and [design_docs/implementation_plan.md](design_docs/implementation_plan.md).

## License and vendored source

mdiff uses the same MIT license as Micro; see [`LICENSE`](LICENSE). The Micro
v2.0.15 source pin, local patch inventory, upstream refresh procedure, and
preserved upstream and third-party notices are documented in
[`design_docs/micro_integration_prototype.md`](design_docs/micro_integration_prototype.md)
and `internal/micro/LICENSE*`. `just audit` checks Go dependencies for known
vulnerabilities and forbidden or unknown licenses; it complements review of the
vendored source notices.
