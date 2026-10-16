# mdiff: Technical Design

This document describes the implemented architecture. The
[future work list](future_work.html) contains the remaining review findings; the
[prototype record](micro_integration_prototype.md) contains the original Micro
source audit and refresh procedure.

## Product contract

mdiff uses Micro's editor behavior in two independently editable, fixed panes.
The panes share one diff-aligned vertical viewport. A Micro action needs an
adapter when it assumes more than two panes or an independent vertical view.
Ctrl+W switches panes; Micro's Open and Save act on the focused pane. A named
file retains its own path and persistence settings. The [README](../README.md)
documents commands, configuration, colors, and known compatibility limits.

## Source boundaries

| Location             | Responsibility                                                                                                                  |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `cmd/mdiff`          | CLI entry point and exit status                                                                                                 |
| `internal/prototype` | Production application, Micro pane integration, event loop, rendering, and file workflows; the package name reflects its origin |
| `internal/compare`   | Bounded line and character comparison, hunk pairing, and immutable diff output                                                  |
| `internal/viewport`  | Paired visual rows, wrap heights, and shared viewport mapping                                                                   |
| `internal/micro`     | Namespaced Micro v2.0.15 source, runtime assets, local integration patches, and upstream notices                                |
| `internal/version`   | The version reported by the CLI and footer                                                                                      |

Micro's upstream packages live under its `internal/` tree, so mdiff cannot
import them directly from another module. The copied source is rehomed below
mdiff's own `internal/micro/` boundary, with imports rewritten. The pinned
revision, original patch inventory, and upstream refresh steps are in the
[prototype record](micro_integration_prototype.md). Keep that subtree's license
and third-party notices with the copied source.

## Pane and terminal lifecycle

The application owns two Micro buffers, two `BufPane` instances, their display
windows, one terminal screen, one comparison coordinator, and one shared
viewport. Each buffer owns its text, cursor, selection, undo history, file
identity, and settings overlay. Micro handles its editing actions, syntax,
keybindings, colorschemes, Lua plugins, prompts, and file persistence. mdiff
routes events to the focused pane or the pane under the pointer and adapts
operations that affect the two-pane layout.

Startup validates arguments before raw terminal mode, loads Micro configuration,
initializes plugins, resolves the selected colorscheme, and constructs both
panes. The default Micro configuration directory is created if missing; an
explicit `--config-dir` must exist. Shutdown stops comparison work, follows
Micro's dirty-buffer and save decisions for both panes, and restores the
terminal. The fixed panes remain synchronized after a Micro command or prompt
replaces a buffer. For `ShellMode` and `Suspend`, mdiff stops and joins the
screen reader before Micro releases the terminal. The shell command uses normal
terminal mode and waits for Enter after showing output; Ctrl+C interrupts the
child command while mdiff retains signal ownership. Resume creates a new screen
and reader, installs any completed diff result through the existing version
check, then redraws the shared two-pane viewport. A restart failure returns an
error after terminal cleanup instead of using the released screen.

## Built-in command boundary

`internal/prototype/app.go` routes registered Micro commands to the focused
pane, except for the ones that remain guarded in this audit. The source
references are in the pinned `internal/micro/action` copy. mdiff has exactly one
Micro tab holding two fixed `BufPane` instances, plus one shared vertical
viewport.

| Command     | Pinned Micro behavior                                                                                                                      | mdiff decision                                                                                                                          |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| `vsplit`    | `VSplitCmd` creates a vertical split and may open file buffers.                                                                            | Block: a third pane has no paired viewport or mdiff owner. `VSplitIndex` guards the missing Micro split tree.                           |
| `hsplit`    | `HSplitCmd` creates a horizontal split and may open file buffers.                                                                          | Block: a third pane has no paired viewport or mdiff owner. `HSplitIndex` guards the missing split tree.                                 |
| `tab`       | `NewTabCmd` creates buffers and calls `Tabs.AddTab`.                                                                                       | Block: an extra tab breaks the fixed two-pane session; `AddTab` rejects mdiff's layout.                                                 |
| `tabmove`   | `TabMoveCmd` removes the active tab and reinserts it at an index.                                                                          | Block: `Tabs.RemoveTab` rejects removal in mdiff, but the command continues to append the tab, duplicating it.                          |
| `tabswitch` | `TabSwitchCmd` selects an existing tab through `Tabs.SetActive`.                                                                           | Block: there is no second tab. A resize creates a new tab with `isActive` false, so selecting it repeats the plugin `onSetActive` hook. |
| `term`      | `TermCmd` starts a terminal and `openTerm` replaces a pane (or creates a tab in a single-pane layout).                                     | Block: replacement loses one of the two editable buffers and its diff owner.                                                            |
| `eval`      | `EvalCmd` only reports `Eval unsupported`.                                                                                                 | Allow: Micro's own visible message has no layout or buffer side effect.                                                                 |
| `log`       | `ToggleLogCmd` calls `OpenLogBuf` or quits the log pane.                                                                                   | Block: `OpenLogBuf` requires a horizontal split; its nil result on mdiff's layout is then dereferenced.                                 |
| `plugin`    | `PluginCmd` opens the log buffer before plugin management.                                                                                 | Block: its log view requires a third pane; plugin commands registered by loaded plugins still use the normal focused-pane route.        |
| `reload`    | `ReloadCmd` calls `reloadRuntime(true)`, replacing runtime files, plugins, settings, bindings, commands, colorscheme, and buffer settings. | Block: the reload is not transactional with mdiff's pane layout, shared viewport, and diff coordinator.                                 |
| `raw`       | `RawCmd` creates a raw-event pane in a new tab.                                                                                            | Block: an extra tab has no paired viewport; `Tabs.AddTab` rejects it.                                                                   |

`reloadRuntime` reads settings and applies them one by one through
`doSetGlobalOptionNative`; `infobar` and `keymenu` invoke `Tabs.Resize`, while
buffer settings reload can change wrap geometry. Neither path calls mdiff's
`App.resize` or rebuilds its current aligned viewport. The routine then resets
bindings and command registration, initializes plugins, and changes the
colorscheme without a rollback path if a later step fails. Also,
`resolveDebounce` validates `mdiff.diff-debounce-ms` against its 40–180 ms rule
only at startup, and the coordinator retains that value in `baseDelay`. Letting
Micro reload settings would therefore show a new debounce setting while existing
and future pending diff timers still use the old delay. A CLI `--diff-debounce`
override would continue to win, but it does not make the rest of Micro's reload
safe. The command remains blocked until a coordinated reload can validate
settings before mutation and update mdiff-owned state and pending timers
atomically. Editing `settings.json` does not change the running coordinator;
restart mdiff to apply it.

## Comparison and visual alignment

Comparison operates on immutable snapshots of both buffers. Every content
mutation changes its side's version. After the configurable debounce, the
coordinator runs at most one active comparison and keeps only the latest pending
request. A result is installed only when both captured versions still match;
stale results are discarded. Large or adversarial comparisons yield a coarse
changed region and warning within the configured work budget instead of blocking
input.

Micro's pinned `go-diff/diffmatchpatch` dependency supplies line-level diff
operations. mdiff pairs replacement lines by index and records additions,
deletions, replacements, and equal lines in an alignment map without inserting
text into either buffer. Changed paired lines also carry character-level rune
spans. Exact equality includes whitespace.

Each aligned logical pair expands to the larger of the two wrapped heights. The
shorter side receives virtual continuation rows; an unpaired line receives
spacer rows on the other side. Both panes render from the same top visual-row
index. Cursor movement, wheel scrolling, diff navigation, mouse clicks, resize,
and wrap changes map through this coordinate. A diff replacement preserves a
content-based viewport anchor where possible and clamps the result to the new
bounds. Horizontal offsets remain pane-local.

## Rendering and colors

Micro's `BufWindow.Display` paints source text into an offscreen tcell surface.
mdiff copies cells into aligned rows, adds virtual spacers, diff backgrounds,
changed-character styles, and the center divider. Selection and the focused
cursor take precedence over diff styling. Equal rows keep Micro's normal colors;
the focused equal line uses Micro's `cursor-line` style. The active cursor is
placed on the real terminal after composition; the inactive pane has no terminal
cursor.

The `mdiff.diff.*` theme groups can override changed and one-sided row colors.
Dark themes use the documented dark defaults; light themes derive pale
backgrounds. A theme that leaves its background to the terminal also leaves row
backgrounds to the terminal, with divider marks retaining coarse-diff
visibility. The [README](../README.md#micro-configuration) lists the groups and
behavior. The visual-position calculation in `sourceRuneLocations` still
duplicates part of Micro's wrap mapping and is tracked in
[future work](future_work.html#wrap-mapping).

## Performance and verification

Comparison must remain outside terminal input handling. The reference profile is
Ubuntu 24.04 on four allocated CPUs and 8 GiB RAM. The measured gates cover a 1
MiB paste becoming visible within 250 ms, a coarse fallback within 2 s for
adversarial input, and a 512 MiB RSS ceiling for the defined 10 MiB fixture. The
exact fixture, environment, measurements, and limitations are in the
[TASK-009 performance report](task_009_performance_report.md). `just bench`
reruns the benchmark suite. `just check` runs the project quality, test, and
build gates; CI also covers supported Linux and macOS targets.
