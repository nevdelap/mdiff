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
replaces a buffer. Terminal handoff for `ShellMode` and `Suspend` is an
[open follow-up](future_work.html#terminal-handoff).

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
