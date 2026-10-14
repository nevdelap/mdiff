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
just run left.txt right.txt
go run ./cmd/mdiff left.txt right.txt
```

`Ctrl+W` switches focus between panes. Edit with Micro's normal movement,
selection, indentation, undo/redo, search, and configured buffer actions. Paste
using the terminal's normal paste shortcut. Micro's defaults use
`Ctrl+Up`/`Ctrl+Down` to move the focused cursor to the start or end of its
buffer and `Alt+Up`/`Alt+Down` to move lines. `Alt-[`/`Alt-]` jump to the
previous or next difference. The mouse wheel scrolls the shared viewport;
`ScrollUp` and `ScrollDown` can also be bound to keys. Wrapping is controlled by
the Micro settings. The visible cursor follows the focused pane and the shared
viewport keeps both panes aligned during edits, scrolling, and resize.

`Ctrl+O` opens a file in the focused pane, `Ctrl+S` saves that pane, and
`Ctrl+T` replaces it with a new unnamed document. Dirty buffers use Micro's
save/discard/cancel prompts, with the pane identified in the prompt. `Ctrl+Q`
quits. The footer shows the supported Micro-style key hints and pane-switch
binding. For a named-path save, open the command bar with `Ctrl+E` and enter
`save <path>`, or bind Micro's `SaveAs` action to a key. Explicit saves write
the selected document.

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
`settings.json`, `bindings.json`, user colorschemes, and syntax definitions and
creates the selected default directory if it is missing. An explicit
`--config-dir` must already exist. Micro's `savehistory`, `savecursor`, and
`saveundo` settings may create files under `buffers/` when used; when both panes
show the same path, the focused pane owns its saved cursor and undo state.
Supported Micro buffer settings apply to both panes, including indentation,
wrapping, ruler options, colors, and configured filetype. `keymenu` and
`statusline` control the footer. Micro's movement, save, open, and quit bindings
reach the focused pane, including user remaps. Bind `DiffPrevious` and
`DiffNext` to choose diff navigation keys; Micro's defaults are `Alt-[` and
`Alt-]`. `ScrollUp`, `ScrollDown`, and related viewport actions move mdiff's
shared viewport. Plugin-defined commands run in the focused pane. Actions that
require extra tabs, panes, or a separate viewport are reported in the status
area. Interactive shell mode remains unavailable because its terminal handoff
has not been integrated with mdiff's event loop.

Diff row backgrounds can be overridden in the selected Micro `.micro`
colorscheme with ordinary `color-link` entries. The built-in dark defaults are:

| Color group                     | Default background |
| ------------------------------- | ------------------ |
| `mdiff.diff.changed.current`    | `#00008B`          |
| `mdiff.diff.changed`            | `#000046`          |
| `mdiff.diff.left-only.current`  | `#8B0000`          |
| `mdiff.diff.left-only`          | `#460000`          |
| `mdiff.diff.right-only.current` | `#006400`          |
| `mdiff.diff.right-only`         | `#003200`          |

For example, `color-link mdiff.diff.changed.current ",#B6D6FF"` sets a
background for the focused changed line. Without an override, light Micro themes
receive pale diff backgrounds blended from their normal background; dark themes
use the colors above. If the theme leaves its background to the terminal, diff
rows keep the terminal background because it may be light or dark; configure the
groups above to color those rows explicitly. On terminal-default themes, the
center divider marks changed rows with `!`, left-only rows with `<`, and
right-only rows with `>` using Micro's diff foreground colors. Equal rows use
Micro's normal colors on both sides, and focused equal lines use Micro's
`cursor-line` color. Syntax, selection, and changed-character foregrounds still
come from Micro's theme.

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

The technical design and open follow-up work are in
[design_docs/micro_diff_editor.md](design_docs/micro_diff_editor.md) and
[design_docs/future_work.html](design_docs/future_work.html). The
[implementation plan](design_docs/implementation_plan.md) indexes active work;
the [prototype findings](design_docs/micro_integration_prototype.md) record the
Micro source integration decision.

## License and vendored source

mdiff uses the same MIT license as Micro; see [`LICENSE`](LICENSE). The Micro
v2.0.15 source pin, original patch inventory, upstream refresh procedure, and
preserved upstream and third-party notices are documented in
[`design_docs/micro_integration_prototype.md`](design_docs/micro_integration_prototype.md)
and `internal/micro/LICENSE*`. `just audit` checks Go dependencies for known
vulnerabilities and forbidden or unknown licenses; it complements review of the
vendored source notices.
