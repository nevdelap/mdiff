# Micro integration prototype findings

## Decision

The TASK-002 prototype is a **GO** for the namespaced Micro source subtree. The
prototype uses the pinned Micro buffer, action, soft-wrap, syntax, and
`BufWindow.Display` implementations while keeping mdiff's alignment and shared
viewport outside Micro. Replacing most of Micro's pane renderer was not
necessary.

The result does not establish that every Micro plugin or full editor command can
be supported. It establishes that the editing and rendering core can be composed
with a diff-aware visual row map while preserving the no-write requirement.

## Prototype findings and later implementation status

- TASK-002 first implemented line-level highlighting; later implementation added
  character-level changed spans. The current renderer uses those spans within
  changed lines.
- The first renderer prototype placed the cursor on its offscreen composition
  surface. The current renderer sets or hides the cursor on the real terminal
  after copying composed cells. Simulation tests cover visible, hidden, and
  resized cursor states; the PTY test checks cursor placement after resize.

## Upstream source and package selection

The source is pinned to Micro v2.0.15 at commit
[`6a62575bcfdf4965f187eedafceb3400316e612b`](https://github.com/zyedidia/micro/commit/6a62575bcfdf4965f187eedafceb3400316e612b).
The copied source closure contains 74 production Go files (about 20,876 lines)
plus tests, runtime data, and the upstream and third-party license notices (290
files total). The closure is placed under `internal/micro/` so mdiff can patch
it without Go's `internal` import restriction: importing upstream's
`github.com/zyedidia/micro/v2/internal/...` packages from
`github.com/nevdelap/mdiff` is prohibited, while the same package code rehomed
below mdiff's own `internal/` tree is legal.

The selected components are:

| Local package                                                | Micro responsibility                             | Prototype use                                                               |
| ------------------------------------------------------------ | ------------------------------------------------ | --------------------------------------------------------------------------- |
| `internal/micro/action`                                      | Buffer actions and keybinding dispatch           | Edits and keyboard actions for each pane                                    |
| `internal/micro/buffer`                                      | Text, cursors, selections, undo, syntax state    | Independent transient buffers                                               |
| `internal/micro/config`                                      | Settings, colorschemes, runtime files            | Micro settings and styles                                                   |
| `internal/micro/display`                                     | Buffer viewport, wrapping, and rendering         | `BufWindow` coordinate conversion and renderer                              |
| `internal/micro/screen`                                      | Micro's tcell screen adapter                     | Terminal lifecycle and cell API expected by Micro                           |
| `internal/micro/pkg/highlight`                               | Syntax file parsing and matching                 | Per-buffer syntax matches                                                   |
| `internal/micro/lua`                                         | Lua runtime used by action/config packages       | Loads Micro plugin initialization and callbacks                             |
| `internal/micro/clipboard`, `shell`, `util`, `views`, `info` | Transitive editor support                        | Build closure; unsafe shell and file actions are filtered in this prototype |
| `internal/micro/runtime`                                     | Built-in syntax, colorschemes, help, and plugins | Built-in assets and bundled Lua plugins                                     |

The Go module keeps Micro's upstream dependency versions, including its
`github.com/sergi/go-diff/diffmatchpatch` dependency. The prototype's line
alignment uses that existing implementation instead of adding another diff
library.

## Rendering experiment

Each side owns a real Micro `buffer.Buffer`, `display.BufWindow`, and
`action.BufPane`. Micro's `BufWindow.VLocFromLoc` determines wrapped-line
coordinates and wrap heights. `BufWindow.Display` paints syntax, selection,
whitespace, cursor-line, and color-column styles into a tcell simulation
surface. The prototype copies those cells into a second composition surface
according to mdiff's shared alignment rows. The final surface is then copied to
the active terminal screen.

The composition layer inserts spacer rows only into the alignment map. It does
not add text to either buffer. For each aligned pair of logical lines, the
layout uses the larger of both wrapped heights and adds continuation spacers to
the shorter side. Both panes index the same top visual row. The adapter maps
cursor positions and preserves scroll anchors in that coordinate space after
edits, resize, or a change in wrap geometry. The cursor is placed on the real
terminal after composition at the focused pane's rendered position.

This arrangement does temporarily swap Micro's global `screen.Screen` while
painting. The terminal event goroutine therefore captures its terminal screen
before the swap and polls that stable reference. Rendering and input actions
remain on the application goroutine. This global state is a known constraint for
later concurrent rendering or multiple mdiff instances in one process.

## Local patches to the vendored Micro source

Import paths throughout the selected source are mechanically rewritten from
`github.com/zyedidia/micro/v2/internal/...` and
`github.com/zyedidia/micro/v2/pkg/highlight` to mdiff's namespaced subtree. The
behavioral patches are:

| File                                | Patch                                                                                                                                                                                                                                        | Reason and effect                                                                                      |
| ----------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `internal/micro/buffer/buffer.go`   | Adds `BTTransient`; skips absolute path resolution, path-glob settings, modification-time checks, cursor/undo recovery, serialization, and backup finalization for that type. Scratch buffers no longer create Micro's `buffers/` directory. | Both panes and the optional info buffer remain memory-only, including during close and interruption.   |
| `internal/micro/action/bindings.go` | Removes `createBindingsIfNotExist` from binding initialization.                                                                                                                                                                              | Reading keybindings must not create or modify Micro configuration files.                               |
| `internal/micro/action/bufpane.go`  | Rejects save, open, quit, shell, command, Lua, tab, and pane-split actions from this prototype's buffer keymap; tolerates mdiff-owned panes without a Micro `TabList`; nil-checks optional `InfoBar` gutter access.                          | Custom Micro bindings cannot leave the in-memory two-pane workflow or invoke unsupported side effects. |
| `internal/micro/action/actions.go`  | Nil-checks `InfoBar` after clipboard paste.                                                                                                                                                                                                  | The standalone prototype does not create Micro's full tabbed info bar.                                 |

No upstream checkout or source repository is modified. The pinned license files
remain with the copied source. To refresh, check out the exact upstream Micro
tag and commit recorded above; copy the production dependency closure, runtime
assets, tests, and notices; re-run the import rewrite; then review and re-apply
the four behavioral patches listed above against the new upstream files. Do not
replace `internal/micro/LICENSE` or `LICENSE-THIRD-PARTY` with mdiff's license
text. Compare the refreshed tree against the previous pin with
`jj diff --summary` and `difft` where available, record the new upstream commit
and changed patch inventory here, and run the vendored package tests, simulation
tests, and PTY checks. This procedure keeps upstream notices with the copied
code and makes patch review repeatable.

## Compatibility limits discovered in the prototype

- Micro settings and colorscheme configuration are loaded. The prototype
  disables Micro bindings that open or save files, quit individual panes, run
  shells or commands, load Lua snippets, or create tabs and splits.
- Micro's info bar is initialized so actions such as undo and redo can report
  status. Command history persistence is disabled for this in-memory session,
  and its pathless scratch buffer does not create the `buffers/` directory.
- Keybindings that map to ordinary buffer actions are read and applied. A
  binding whose action is unsupported is ignored by the prototype's filtered
  action map. `F6` is reserved by mdiff for pane focus; Micro's `Tab` binding
  remains available for indentation and completion.
- Lua is initialized and configured plugins load from Micro's normal locations.
  Compatibility is limited to APIs available in the two-pane session: plugins
  that require tabs, extra panes, independent vertical views, or writable
  configuration can fail or report the operation unavailable. Plugins are
  trusted code and are not sandboxed.
- Buffer actions report through the initialized in-memory `InfoBar`. The two
  panes suppress their status lines and use an mdiff footer.
- Micro's renderer owns per-window horizontal state. The prototype keeps that
  state pane-local, while vertical position and wrap alignment are shared by
  mdiff.

## Measured prototype surface

The disposable prototype is isolated in `internal/prototype/`; its alignment and
visual row mapping are in `internal/compare/` and `internal/viewport/`. The
Micro renderer adapter is 116 lines; the application lifecycle and event adapter
is 635 lines, while alignment and shared viewport mapping total 195 lines. This
surface includes the tcell composition bridge and does not replace Micro's
903-line `BufWindow` renderer. It exercises independent empty buffers, bracketed
paste, keyboard editing, wheel scrolling, shared vertical navigation, diff
jumps, resize, wrapped lines, edit-anchor restoration, normal exit, and
interrupt cleanup. PTY tests synchronize on rendered terminal state, verify
scroll input from both focused panes, check visible cursor placement after
resize, and confirm restoration and an empty config directory after interruption
following an edit. The config-directory test verifies that close creates no
settings, bindings, recovery, or buffer files.

The prototype keeps full-buffer line snapshots and recalculates alignment
synchronously after each input event. This validates correctness and integration
boundaries, not the production latency target for very large pastes. Debounced
or incremental comparison and large-input benchmarks remain later work.
Character-level changed-span computation remains deferred follow-up work. The
visible-cursor issue found in manual use was resolved during review and is now
covered by simulation and PTY checks.
