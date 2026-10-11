# Implementation Plan

This file is the source of truth for implementation task scope, dependencies,
acceptance criteria, and state. The architecture and proposed phases are
described in [`micro_diff_editor.md`](micro_diff_editor.md). The first two tasks
establish a pinned Micro integration baseline, then validate the diff-aware
scroll architecture before full application implementation.

## Tasks

### TASK-001 - Pin Micro and audit prototype feasibility

State: COMPLETED

Goal:

- Select and document an exact Micro upstream revision and determine the
  smallest realistic integration surface for the synchronized-scroll prototype.

Dependencies:

- The architecture and prototype objectives in
  [`micro_diff_editor.md`](micro_diff_editor.md).
- No application implementation task may start until this task is complete.

Scope:

- Inspect the selected Micro revision's module and Go version, package
  dependency graph, buffer construction and teardown, action dispatch, tcell
  simulation screen, paste and resize event path, and `diffmatchpatch` API.
- Trace all built-in buffer persistence paths reachable from construction,
  editing, close, and shutdown. Identify a concrete transient-buffer strategy
  and the tests needed to prove that buffer text is never serialized or written.
- Record the upstream SHA/version, applicable license and notice requirements,
  source subtree/import rewrite strategy, required local patches, and an update
  procedure. Confirm how Go `internal/` imports will remain within mdiff's
  module boundary.
- Decide the default plugin policy for the prototype, which must not load
  plugins. Apply the product policy already recorded in the design: plugins are
  disabled by default, and an explicit `--enable-plugins` opt-in places plugin
  filesystem/process effects outside mdiff's built-in no-write guarantee. Defer
  full settings, keybinding, color scheme, syntax, and plugin compatibility to
  later tasks.
- Update the Micro implementation findings and Phase 0 decision in
  [`micro_diff_editor.md`](micro_diff_editor.md). Record unresolved risks and an
  explicit go/no-go recommendation for TASK-002.

Out of scope:

- Copying or modifying upstream Micro source, implementing the editor, or
  changing the application scaffold.
- Enabling plugins or promising compatibility with user configuration.
- Changing the product goals or replacing the shared visual coordinate model.

Acceptance criteria:

- The design document names an immutable upstream Micro revision and lists the
  exact upstream files/packages and license notices relevant to the prototype.
- A dependency/import diagram or equivalent written trace shows how the selected
  Micro code can compile under mdiff without violating `internal/` import rules;
  the expected subtree and import rewrite boundaries are listed. The documented
  layout is compile-probed, and the task verifies the actual selected Micro
  dependency closure against it before any source is copied.
- Every built-in path from transient buffer creation through normal exit is
  accounted for, with a concrete no-write implementation and test plan. Any
  unproven path is explicitly marked as a blocker rather than assumed safe.
- The selected tcell simulation and PTY event routes are identified for paste,
  wheel/keyboard scrolling, cursor input, and resize.
- The task concludes with a justified `GO` or `NO-GO` recommendation for the
  integration and scroll prototype. A `NO-GO` result must propose the smallest
  alternative prototype that can answer the same architectural questions.
- Documentation formatting and lint checks pass.

Verification:

- `just format`
- `just lint`
- Any source inspection or temporary compile probes are recorded with their
  exact commands and upstream revision; do not add probe artifacts to the
  product tree unless needed by TASK-002.

Handoff:

- Pinned Micro `v2.0.15` at `6a62575bcfdf4965f187eedafceb3400316e612b` and
  recorded the module path, Go version, focused package closure, source volume,
  MIT and third-party notice requirements, no-write hazards, Lua/plugin startup
  constraint, terminal event routes, diff API, proposed local patches, and
  upstream update procedure in [`micro_diff_editor.md`](micro_diff_editor.md).
- Verified the focused source closure as 11 `internal` packages plus
  `pkg/highlight` and `runtime`; the source is 74 non-test Go files / 20,876
  lines before runtime assets. Rehoming the source under
  `github.com/nevdelap/mdiff/internal/micro` and rewriting the upstream import
  prefix compiled in a disposable `/tmp` probe.
- The documented probe commands include `go mod download -json`,
  `go list -deps`, and `go build` for all 13 rehomed packages. The temporary
  probe and module caches are outside the product tree; no upstream or product
  source was changed.
- Recommendation: **GO to TASK-002**, gated on proving the shared-row renderer
  and transient no-write path in the prototype. TASK-002 remains `NEW` and was
  not started.
- Documentation checks passed after final edits: `just format` and `just lint`
  (including `go vet`, `golangci-lint`, Markdownlint, and yamllint). No
  application tests were run because TASK-001 changes only design documents.
- `jj status` could not snapshot the working copy because the managed
  environment mounts `.git/objects` read-only; no Git fallback was used.

### TASK-002 - Prototype Micro integration and shared diff scrolling

State: COMPLETED

Goal:

- Build a minimal, disposable product-tree prototype that proves real Micro
  editing can render through one shared diff-aware vertical coordinate system.
  The result decides whether the recommended namespaced Micro subtree remains
  viable before full editor and configuration work begins.

Dependencies:

- TASK-001 is `COMPLETED` with a `GO` recommendation.
- Use only the pinned Micro revision and integration boundary recorded by
  TASK-001.

Scope:

- Add the smallest Micro source subtree and import rewrites required to compile
  and run two independent, empty, in-memory buffers under mdiff. Preserve
  upstream notices and document every local Micro patch. No source change is
  made to the upstream checkout.
- Add a prototype entry point or explicit prototype mode that starts one tcell
  screen and renders two editable side-by-side Micro panes. It must not accept
  file operands, load plugins, save, serialize, back up, or recover buffer
  contents. Keep prototype-only code isolated so it can be retained or removed
  cleanly after the architecture decision.
- Implement line-level comparison using the pinned Micro revision's existing
  `diffmatchpatch` dependency where feasible. Convert operations into an ordered
  alignment map with virtual spacer rows; do not insert spacer text in either
  Micro buffer.
- Implement one canonical expanded visual-row coordinate shared by both panes.
  It must account for unmatched logical lines and unequal soft-wrap heights.
  Scrolling from either pane changes the same top visual row. Cursor movement
  keeps the active cursor visible through the shared mapping, and edits that
  alter alignment above the viewport preserve a logical/context anchor.
- Support focused-pane switching, keyboard navigation and scrolling, terminal
  bracketed paste into either pane, and terminal resize. Mouse-wheel scrolling
  should be wired when the selected tcell event path permits it; document any
  terminal/protocol limitation.
- Add deterministic simulation-screen tests and a PTY smoke test. Exercise
  insertion on one side, deletion on the other, virtual spacer rendering,
  unequal pane widths with wrapping, scroll input over either pane, cursor
  movement across a hunk, an edit above the viewport, resize, paste, and
  no-write shutdown.
- Record measured findings and the `GO`/`NO-GO` architecture decision in this
  plan and the Micro integration section of the design document. If adapting
  Micro rendering requires replacing most of `BufWindow`, stop expanding the
  prototype and document the evidence and fork-level alternative.

Out of scope:

- Full production editor behavior, undo/redo acceptance, normal Micro
  configuration loading, custom keybinding support, colorscheme/syntax
  compatibility, or Lua plugins.
- Debounced asynchronous diffing, incremental diff algorithms, intraline
  highlighting, large-input optimization, comprehensive hunk navigation, and
  release packaging.
- Save/open/file arguments or any persistence of buffer contents.

Acceptance criteria:

- The prototype builds as a standalone Go command and starts with two empty
  editable panes. It uses actual Micro buffer/action/rendering code from the
  pinned revision rather than a mock editor.
- Each side's edits affect only its own in-memory buffer. Bracketed paste is
  accepted in both panes. Closing normally and interrupting do not create or
  modify files containing buffer contents, backups, or recovery state.
- A testable alignment representation includes real lines and virtual spacer
  rows in source order. Rendering both panes at any shared visual coordinate
  produces corresponding alignment rows; neither pane has an independently
  authoritative vertical offset.
- Simulation-screen tests demonstrate that scrolling from either pane keeps
  paired logical lines aligned, including across insertion and deletion rows.
- With soft wrapping enabled and unequal text widths, paired alignment records
  consume the maximum wrapped height on both sides. The shorter side renders
  blank continuation cells/rows. Resize rebuilds the mapping without losing the
  viewport anchor or hiding the active cursor.
- Scrolling and keyboard cursor navigation across a hunk do not accumulate
  vertical drift. An edit above the viewport retains the anchored logical
  content where it remains identifiable; an ambiguous anchor uses and tests a
  deterministic fallback.
- PTY smoke coverage confirms paste, at least one scroll from each side, resize,
  normal exit, and terminal restoration. The simulation suite covers
  wrap/alignment and anchor cases that a PTY cannot reliably inspect.
- `just format`, `just lint`, `just test`, `just build`, and `just test-pty`
  pass on Linux. The simulation and build/test gates also pass on macOS in CI;
  PTY coverage runs on macOS if the selected harness supports it.
- The task handoff records source volume copied, local patch count, renderer
  adaptation size, limitations, and a justified decision to proceed with or
  replace the recommended architecture.

### Deferred findings recorded after TASK-002

- Character-level diffing is a product requirement. Diff rendering must mark
  changed character spans within replacement lines; line-level hunk coloring
  alone does not satisfy it. Preserve the existing virtual-row line alignment
  and underlying Micro buffer contents. Cover insertions and deletions within a
  line, Unicode text, and transitions between changed and unchanged lines.
- Manual use found no visible terminal cursor. This review finding is resolved:
  the active cursor is now applied to the real terminal after cell composition,
  with simulation coverage for visible, hidden, and resize cases and PTY
  coverage for the cursor position after resize.

Verification:

- `just format`
- `just lint`
- `just test`
- `just build`
- `just test-pty`
- Linux and macOS CI jobs for all supported simulation/build tests

Handoff:

- Copied the pinned Micro `v2.0.15` source closure under `internal/micro/`: 74
  production Go files / about 20,876 lines, 13 package/runtime areas, plus
  tests, embedded runtime assets, and upstream license notices. Import paths
  were rewritten to comply with Go's `internal/` rule. No upstream checkout was
  modified.
- Added four behavioral patch files: transient no-write buffers, read-only
  binding initialization, unsafe-action filtering and optional `InfoBar` guards.
  The exact patch inventory and compatibility limits are in
  [`micro_integration_prototype.md`](micro_integration_prototype.md).
- Confirmed Micro rendering adaptation does not require replacing `BufWindow`:
  `BufWindow.Display` renders each source line into an offscreen tcell surface,
  then mdiff maps its cells onto virtual spacers and shared wrapped visual rows.
  The adapter is 116 lines; the alignment and viewport packages total 195 lines,
  compared with 903 lines in Micro's existing `BufWindow` renderer.
- The prototype covers two independent editable buffers, bracketed paste, shared
  keyboard/mouse scrolling, diff jumps from either side, soft wrapping,
  resizing, edit-anchor preservation, and no-write shutdown. PTY tests
  synchronize on rendered state, verify keyboard scrolling from both focused
  panes, check an in-bounds cursor after resize, and verify terminal restoration
  and a clean config directory after normal exit and interruption following an
  edit.
- Architecture decision: **GO** to continued implementation with the namespaced
  Micro subtree and the shared visual-row model. Do not treat this prototype as
  proof of plugin compatibility or large-input performance; both remain
  explicitly deferred.
- Follow-up notes from manual use after TASK-002: changed-line highlighting is
  currently line-level only, but the product requirement is character-level
  diffing within changed lines. The missing visible cursor was a review finding
  and is resolved by applying its position to the real terminal after cell
  composition.
- Linux `just format`, `just lint`, `just test`, `just test-pty`, and
  `just build` passed. `go run -buildvcs=false ./cmd/mdiff --version` printed
  the pinned prototype version. macOS verification is delegated to the existing
  CI matrix and has not been run locally.
