# Implementation Plan

This file is the source of truth for implementation task scope, dependencies,
acceptance criteria, and state. The architecture and proposed phases are
described in [`micro_diff_editor.md`](micro_diff_editor.md). TASK-001 pins and
audits Micro integration; TASK-002 validates the shared-scroll prototype.
TASK-003 through TASK-011 specify the remaining product implementation and
release work.

## Product direction — owner decision, 2026-10-11

**mdiff should work like Micro in every way except for its two-pane
specialness.** Micro is the behavior baseline, not a selective source of editing
components. The only intended product-level difference is that two active panes
are shown and edited together in one diff-aligned viewport. Preserve Micro's
file open and save behavior, per-pane filenames, keybindings, prompts, status
lines, two-row comma-separated key menu, settings, colorschemes, syntax, and
other ordinary editor behavior. Adapt a feature only when it conflicts with the
two-pane diff model; do not retain earlier restrictions merely because they were
part of the prototype.

Maximize reuse of Micro's existing code and minimize code differences from the
pinned Micro source. Keep mdiff-specific patches small, explicit, and limited to
integration needs such as the shared diff viewport, paired panes, per-pane file
identity, and mdiff-owned pane switching. Prefer adapting Micro extension points
over copying or replacing editor behavior.

This decision supersedes the earlier memory-only/no-write product policy and any
task text below that forbids Open, Save, or ordinary Micro behavior. The
implementation tasks must be read and updated against this contract. The
pane-switch key is Ctrl+W (shown as `^W Switch Panes`); Micro's Ctrl+S Save and
Ctrl+O Open operate on the focused pane.

In the two-row key menu, put the focused `Left`/`Right` indicator at the right
edge of row one. Put `mdiff (wrapping micro) v0.0.1` at the right edge of row
two, using the same real version value that `--version` reports.

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

### TASK-003 - Render character-level differences

State: COMPLETED

Goal:

- Extend line-level comparison so replacement rows show the differing characters
  within each line, while preserving the shared line alignment and leaving both
  Micro buffers unchanged.

Dependencies:

- TASK-002 is `COMPLETED` and its shared-row architecture is approved.
- Use the existing `github.com/sergi/go-diff/diffmatchpatch` dependency pinned
  through Micro; do not add another diff dependency without measured evidence.

Scope:

- Work primarily in `internal/compare` for rune diff operations and span types,
  and `internal/prototype/render.go` for display-cell mapping and styling.
  Extend `internal/viewport` row data only if the immutable render model
  requires it.
- Add immutable per-side change spans to paired `Changed` rows. Define spans in
  Unicode rune offsets, with explicit conversion to Micro display columns for
  rendering tabs, wide runes, and combining marks. Expand styling to a complete
  terminal cell when a rune occupies multiple cells or a combining sequence.
- Compute intraline operations for paired replacement lines. A line paired
  against a virtual spacer is wholly added or deleted; equal rows receive no
  character change spans. Keep line operation classification and row order
  unchanged.
- Render distinct added and deleted character styles using configured diff
  styles and the explicit cell-style composition order in
  [`micro_diff_editor.md`](micro_diff_editor.md#cell-style-composition). Missing
  diff styles must use documented defaults.
- Keep comparison outputs immutable and independent of live Micro buffers.

Out of scope:

- Debouncing, workers, incremental comparison, large-input tuning, and viewport
  redesign; those are specified in later tasks.
- Editing either buffer, inserting markers into buffer text, or changing Micro
  undo state to represent diff spans.

Acceptance criteria:

- Unit tests cover identical lines, one insertion, one deletion, multiple
  disjoint edits, empty lines, replacement pairs, and wholly added/deleted
  lines. Equal text yields no intraline spans.
- Unicode tests cover non-ASCII runes, wide terminal characters, combining
  marks, and tabs. Span-to-cell conversion neither omits changed visible cells
  nor styles a partial wide character.
- Renderer tests show added/deleted character styles on the correct side while
  retaining line-level hunk context, syntax colors, selections, and the visible
  cursor. Include a selected changed span and a focused cursor on a changed cell
  to verify precedence. Virtual spacer rows remain blank and do not receive
  fabricated text.
- Property tests verify spans reconstruct each paired source line when applied
  with equal spans and side-specific insert/delete spans. Neither buffer text,
  cursor, selection, undo, nor redo state changes during comparison/rendering.
- `just format`, `just lint`, `just test`, and `just build` pass.

Verification:

- `just format`
- `just lint`
- `just test`
- `just build`

Handoff:

- Record the span coordinate convention, style precedence, Unicode display-cell
  behavior, test coverage, and any `diffmatchpatch` limitations. TASK-004 must
  preserve the immutable character spans when moving comparison off the input
  path.
- Implemented half-open Unicode-rune `ChangeSpan` ranges from Micro's pinned
  `diffmatchpatch` rune operations. Changed line pairs carry paired left/right
  spans; unpaired spacer rows carry a whole-line span on their real side, and
  equal rows carry none. Comparison and rendering do not mutate source buffers.
- Corrected operation counting for `DiffLinesToRunes` output to count encoded
  line tokens as runes, with a regression test inserting a line after the token
  table has crossed the one-byte UTF-8 range.
- Propagated spans through the shared visual-row layout. The renderer maps
  source runes to tcell display cells in a single pass matching Micro's tab,
  wide-rune, word-wrap, soft-wrap, and horizontal-scroll coordinates. Combining
  marks share their base grapheme's changed/selection style. Configured
  `diff-added`/`diff-deleted` styles override syntax and hunk tint; missing
  styles use green/red foreground and inherit the hunk background. Selection
  styles take precedence over diff styles, then the focused cell receives
  reverse cursor treatment.
- Added comparison unit cases and a seeded fuzz property that reconstructs each
  side from paired spans and equal context; renderer tests cover syntax/hunk
  composition, add/delete styles, selection and cursor precedence, tabs, wide
  and combining runes, Micro wrap-coordinate parity, and textless virtual
  spacers. They also assert rendering leaves both buffers, cursors, selections,
  and undo/redo stacks unchanged.
- Verification passed: `just format`, `just lint`, `just test`, and
  `just build`. The comparison fuzz target also passed for 5 seconds with
  `GOMAXPROCS=2`. `just lint` required installing the repository-pinned
  `golangci-lint` v2.14.0 via `just install-tools`; the final lint run reported
  zero issues. The full test suite and build passed with
  `GOCACHE=/tmp/mdiff-task3-go-cache`.

### TASK-004 - Add versioned debounced diff updates

State: COMPLETED

Goal:

- Move full-buffer line and character comparison off the terminal input path and
  install only results that match the current left and right buffer versions.

Dependencies:

- TASK-003 is `COMPLETED`, with immutable line and character diff output.
- Use Micro's pinned buffer mutation/event boundary and the shared-row model
  validated by TASK-002.

Scope:

- Add the coordinator and deterministic test seams under `internal/prototype`,
  with mutation-version notifications at `internal/micro/buffer` and immutable
  comparison inputs/outputs in `internal/compare`.
- Track a monotonic content version for each buffer. Emit a coalescible change
  notification for typing, paste, undo, redo, deletion, and any supported
  command that mutates text. Do not rely on cursor movement or a render-frame
  dirty flag as a change signal.
- Capture immutable snapshots on the application goroutine. Workers must never
  read live Micro buffers. Read `diff-debounce-ms` from the mdiff-namespaced
  settings file or `--diff-debounce` CLI option (CLI wins); default to 80 ms and
  accept configured values only from 40 through 180 ms. Start/restart the timer
  after every coalesced text mutation, so the delay is measured from the most
  recent mutation. For a paste event of at least 256 KiB of inserted UTF-8 text
  or at least 10,000 inserted newline characters, use
  `min(configuredDelay+20 ms, 200 ms)` for that pending batch. Smaller pastes
  use the configured delay. Navigation and non-text events do not restart it.
- Permit at most one active comparison and one newest pending snapshot pair.
  Cancel superseded work when supported, discard stale version pairs, and
  immediately schedule the latest pair after an obsolete job completes.
- Keep the last valid map on screen and indicate that it is stale while a new
  comparison is pending. Install a result, remap the viewport anchor, and redraw
  both panes as one event-loop operation.
- On shutdown, stop timers, prevent new submissions, and join or cancel workers.
  Surface comparison errors without corrupting the last valid map.

Out of scope:

- Incremental diff algorithms, Fenwick trees, and changing the pinned diff
  library. Optimize only after TASK-009 benchmarks justify it.
- Changes to plugin policy or Micro configuration compatibility.

Acceptance criteria:

- Deterministic unit tests with an injected clock and worker prove debounce
  bounds, coalescing, one-active/one-pending limits, stale-result rejection,
  correct version pairing, latest-result installation, error handling, and
  shutdown without leaked goroutines. Cover 40 ms, default 80 ms, 180 ms, paste
  threshold boundaries, the 200 ms cap, and repeated edits resetting the timer.
- Mutation tests prove every supported text-changing path advances only the
  owning buffer's version, including undo, redo, and bracketed paste. Navigation
  without edits does not advance versions.
- Integration tests paste and edit a large fixture while input remains
  responsive, show a stale indicator during work, and converge to an alignment
  matching the final snapshots.
- No worker accesses a live Micro buffer; race-enabled tests pass for the
  coordinator and its test doubles.
- `just format`, `just lint`, `just test`, `just test-race`, and `just build`
  pass.

Verification:

- `just format`
- `just lint`
- `just test`
- `just test-race`
- `just build`

Handoff:

- Record debounce measurements, shutdown behavior, queue bounds, and any
  cancellation limitation. Include a trace or test proving that an obsolete
  result cannot replace a newer alignment.
- Implemented `EventHandler` change notifications and monotonic per-buffer
  versions. Successful insertion, deletion, replacement, undo, redo, search
  replacement, and Micro action edits report through the shared mutation
  boundary; cursor navigation does not change versions. Notifications include
  inserted UTF-8 bytes and newline counts so bracketed paste can select the
  large-paste delay.
- Added a single app-owned coordinator with an injectable timer and comparison
  function. It uses an 80 ms default, accepts 40–180 ms from the `mdiff` object
  in Micro `settings.json`, and honors `--diff-debounce` over that setting.
  Large paste events (at least 256 KiB or 10,000 inserted newlines) add 20 ms,
  capped at 200 ms. Every mutation resets the timer; navigation leaves it alone.
- The app goroutine captures detached line strings and version pairs after the
  timer fires. A worker runs the pinned line and rune comparison over those
  snapshots only. At most one comparison runs and one newest snapshot waits. The
  pinned comparison API has no cancellation hook, so running work is allowed to
  finish; a mismatched result is discarded and the newest pending pair starts
  immediately. A regression test verifies that when `v2` is pending and a new
  edit creates `v3`, the obsolete active result triggers a fresh `v3` capture
  and `v2` never starts.
- The last valid alignment remains visible with a `Diff updating…` footer while
  work is pending. Its immutable displayed snapshots keep viewport anchors tied
  to the visual map even if edits shift live line numbers. A matching result
  swaps the pairs, displayed snapshots, anchor mapping, and expanded layout on
  the event-loop goroutine. Errors retain the last map and show a footer error.
  Shutdown stops the timer and joins the running finite comparison before
  closing Micro buffers and restoring the terminal.
- `just test-race` initially exposed races in Micro's asynchronous syntax path.
  Syntax workers now compute against detached line snapshots and publish state
  only if the captured buffer version and highlighter still match. The shared
  buffer lock serializes text events with snapshot capture and style install; an
  atomic content version changes inside the same text-event lock. Large changed
  ranges defer synchronous rehighlighting to this snapshot worker so a follow-up
  keystroke does not wait for a full large-line scan. `LineArray.Bytes` indexes
  each line's data instead of copying whole `Line` values and their
  asynchronously updated syntax fields. Syntax highlighting stays enabled, and
  the complete race suite passes.
- Added buffer mutation/version tests; deterministic fake-timer tests for
  debounce boundaries, timer resets, coalescing, stale result rejection,
  snapshot ownership, errors, and shutdown; app tests for navigation, anchor
  stability, and a 256 KiB paste that converges to the final snapshots; and
  retained the PTY paste/scroll/resize/terminal-restoration test.
- Verification passed: `just format`, `just lint`, `just test`,
  `just test-race`, and `just build`. The first race run exposed the Micro
  syntax worker race, and the large-fixture test also measured a 1.55 s delay on
  a follow-up edit while syntax work overlapped. Detached syntax snapshots,
  synchronized metadata publication, and deferred large-range rehighlighting
  resolved both issues. The final 256 KiB paste-and-edit test kept each event
  under its 2 s/1 s responsiveness bounds, used the configured paste delay plus
  20 ms, showed the stale footer, and installed an alignment matching final
  snapshots.

### TASK-005 - Complete Micro editing behavior

State: COMPLETED

Goal:

- Validate and complete ordinary Micro editing behavior in both panes. TASK-011
  supplies the per-pane file lifecycle so these remain normal Micro buffers.

Dependencies:

- TASK-002 is `COMPLETED`.
- TASK-004 is `COMPLETED` so every supported edit can publish the required
  mutation/version notification.
- Micro action dispatch and buffer APIs remain pinned to the audited revision.

Scope:

- Work in `internal/prototype` and the required pinned packages under
  `internal/micro/action` and `internal/micro/buffer`; preserve vendored notices
  and keep changes inside mdiff's module.
- Exercise and repair supported Micro actions for cursor movement, selections,
  cut/copy/paste, indentation and unindentation, autocomplete where usable,
  undo/redo, search within the current in-memory buffer, and ordinary text
  editing. Preserve independent cursor, selection, and undo history per pane.
- Ensure all edits pass through the mutation/version hook from TASK-004 without
  splitting paste or compound edits into surprising undo groups.
- Keep open/save/save-as/reload unavailable in TASK-005; per-pane Open and Save
  are specified in TASK-011. Shell, suspend, and external command actions remain
  unavailable. A rejected configured action must produce a concise user-visible
  explanation without changing either buffer.
- Add a Micro-style per-pane status line above a two-row key menu. Show each
  pane's filename or `No name`, cursor position as `(line,column)`, filetype as
  `ft:<type>`, line-ending mode, and encoding. Keep transient prompts and
  messages visible in their own shared status area.
- Put `^W Switch Panes` first in the key menu, then preserve Micro's exact
  key-menu order and comma separators: row one is
  `^Q Quit, ^S Save, ^O Open, ^G Help, ^E Command Bar, ^K Cut Line`; row two is
  `^F Find, ^Z Undo, ^Y Redo, ^A Select All, ^D Duplicate Line, ^T New Tab`.
  TASK-011 implements Open and Save for the active pane. Adapt New Tab to the
  exactly-two-pane model rather than creating a third pane. TASK-006 owns user
  binding remaps and must update labels from effective bindings.
- Reserve both rows for the key menu and avoid collisions or wrapping into pane
  content at narrow terminal widths. Keep per-pane status and cursor rendering
  distinct from the shortcut bar. Show only this one permanent key-hint menu;
  keep the separate status row available for transient messages and prompts, but
  remove the permanent F6/scroll/difference controls legend from it. Ensure
  Ctrl+W switches panes through the actual terminal input path and appears first
  as `^W Switch Panes`. Put the current Left/Right pane indicator at the far
  right end of the first key-menu row and `mdiff (wrapping micro) v0.0.1` at the
  far right of row two. The footer and `--version` use the same real version
  source. Function keys are not required.

Out of scope:

- Full Micro plugin compatibility, which is covered by TASK-007.
- Redesigning diff algorithms or the shared vertical coordinate.

Acceptance criteria:

- Simulation-screen integration tests run supported built-in editing actions in
  each pane and prove that edits, selection, undo, redo, and search state do not
  leak to the other buffer. User-configured binding behavior is tested in
  TASK-006 after config loading is implemented.
- Tests cover built-in ordinary editing actions and confirm unsafe actions fail
  clearly without writing files, changing buffer names, or terminating the
  application. Tests also confirm conflicting configured unsafe bindings on
  Ctrl+Q, Ctrl+G, and Ctrl+E are reported without running the app-owned action,
  while the default Ctrl+Q still exits. Ctrl+W switches panes. TASK-011 covers
  Ctrl+S Save and Ctrl+O Open for the focused pane.
- Simulation-screen tests verify two per-pane Micro-style status lines and the
  two-row key menu with comma separators, exact label order, and no overlap at
  narrow widths. Each status line shows the current filename or `No name`,
  cursor position, filetype, line-ending mode, and encoding. `^W Switch Panes`
  precedes Micro's unchanged key-menu sequence, and the current Left/Right pane
  indicator appears at the far right of row one; the product/version label is at
  the far right of row two and matches `--version`. Transient messages and
  prompts remain visible. PTY tests verify Ctrl+W switches focus. TASK-011
  covers the Open/Save menu actions.
- Bracketed paste preserves exact text and newline semantics, remains one
  coherent undo operation where Micro supports it, and updates the diff after
  the mutation version changes.
- The terminal cursor remains in the focused pane after movement, selection,
  undo/redo, and focus changes. Normal exit and SIGINT after edits leave the
  configured directory free of buffer, backup, and recovery state.
- `just format`, `just lint`, `just test`, `just test-pty`, and `just build`
  pass.

Verification:

- `just format`
- `just lint`
- `just test`
- `just test-pty`
- `just build`

Handoff:

- List supported and rejected actions, key sequences reserved by mdiff, and any
  Micro behavior that cannot be preserved. Update the compatibility matrix only
  with behavior covered by tests.

Implementation handoff:

- Kept Micro's independent `BufPane` cursors, selections, action dispatch, and
  undo stacks. Simulation tests exercise movement, select all, copy/cut/paste,
  cut and duplicate line, indentation/outdentation, in-buffer autocomplete,
  search, undo/redo, focus changes, and bracketed paste's exact newline content
  and single-operation undo/redo in both transient panes.
- Routed Micro's InfoBar prompt input through the app and rendered it on a
  dedicated status row, with its cursor shown on that row. Ctrl+F therefore
  performs interactive search. Ctrl+E opens a restricted Micro-style command
  bar; only `goto`, `jump`, `replace`, `replaceall`, `retab`, `show`, `showkey`,
  and `memusage` are accepted. Ctrl+G displays concise mdiff help without
  creating a third buffer. Rejected commands show a status message.
- Added a Micro-style status line for each pane, a shared transient status row,
  and the two-row comma-separated Micro key menu. The menu prefixes Micro's
  standard order with `^W Switch Panes`, puts the focused pane indicator at the
  right of row one, and the product/version label at the right of row two. The
  footer and `--version` share the application version constant. Narrow
  terminals truncate menu text without wrapping or overlapping the right-side
  indicators.
- `Ctrl+Q` exits; `Ctrl+E` and `Ctrl+G` are mdiff-owned command/help bindings;
  `Ctrl+W` switches panes, while `Ctrl+Up/Down` and `Alt+Up/Down` scroll and
  navigate differences. Conflicting configured unsafe actions and rejected
  sequence prefixes on mdiff-owned keys produce a visible explanation without
  running either action; default Ctrl+Q still quits. Save, Save As, Open, shell,
  suspend, tab, split, help-buffer, and external command actions are filtered in
  TASK-005. TASK-011 restores per-pane Open, Save, and New Tab behavior.
- The Micro command bar intentionally supports a safe subset rather than
  arbitrary Micro commands. The help key provides short inline usage guidance
  instead of opening Micro's help buffer. External clipboard tools are not
  invoked; in-memory clipboard actions and terminal bracketed paste are tested.
- `just format`, `just lint`, `just test`, `just test-pty`, `just build`, and
  the repository Go vet and golangci-lint gates passed after the review fixes.
- TASK-005 preserves Micro's pinned default line-number setting in both panes;
  TASK-006 verifies broader settings and user-configured `ruler` behavior,
  binding-aware menu labels, and active-colorscheme menu colors.

### TASK-011 - Implement Micro-compatible per-pane file workflows

State: COMPLETED

Goal:

- Give each pane Micro-compatible file identity and open/save behavior while
  preserving the exactly-two-pane diff layout.

Dependencies:

- TASK-005 is `COMPLETED` so editing, focus, and the Micro key menu are stable.
- The owner direction at the top of this plan supersedes the prototype's
  memory-only/no-write policy.

Scope:

- Accept zero, one, or two file arguments before terminal initialization. Map
  one argument to the left pane and two arguments left-to-right. Reject more
  than two paths before entering raw terminal mode.
- Give each pane an independent path, display name, dirty state, filetype,
  encoding, and line-ending mode. Opening or saving one pane must never change
  the other pane's text or filename.
- Implement Micro's `Ctrl+O Open` and `Ctrl+S Save` actions on the focused pane.
  Open prompts for a path and loads into that pane; Save writes to that pane's
  existing path or prompts for a path when it is unnamed. Preserve the existing
  file on open/save errors and report the failure through Micro's status UI.
- When Open or New Tab would replace modified content, use Micro-compatible
  Save/Discard/Cancel handling. Adapt `Ctrl+T New Tab` to create a new unnamed
  document in the focused pane without creating a third pane.
- Reuse Micro's file buffer and explicit open/save persistence paths, including
  file metadata and the behavior Micro provides under its default settings. Do
  not maintain a parallel mdiff file format or custom persistence stack without
  a documented Micro limitation. TASK-011 does not load or promise compatibility
  with user-configured backup/autosave settings; TASK-006 tests those settings
  against these file-backed pane fixtures after configuration loading exists.
- Render each pane's Micro-style status line above the two-row, comma-separated
  key menu. Show filename or `No name`, `(line,column)`, `ft:<type>`,
  line-ending mode, and encoding. Put the current Left/Right pane indicator at
  the far right of row one. Put `mdiff (wrapping micro) v0.0.1` at the far right
  of row two. Define one real application version value, currently `v0.0.1`, and
  use it for this label and `--version`.
- Keep shell, suspend, and other Micro behavior unless it conflicts with the
  two-pane model. Document any remaining deviation with evidence and an explicit
  owner decision.

Out of scope:

- Creating a third pane or abandoning shared diff-aligned vertical scrolling.
- Replacing Micro's persistence implementation with a separate mdiff format.

Acceptance criteria:

- PTY and simulation-screen tests prove Ctrl+O opens into the focused pane and
  Ctrl+S saves only that pane. Ctrl+W changes focus and never saves. Switching
  focus changes the active filename/status data without cross-pane leakage.
- Startup tests cover zero, one, and two file arguments, plus rejection of more
  than two paths before raw terminal mode.
- Tests cover an unnamed pane, independent filenames, clean and dirty Open,
  Save/Save As, New Tab adaptation, nonexistent/unreadable paths, write errors,
  and preservation of the prior buffer/path after failure. Persistence tests use
  Micro's default configuration and verify explicit Open/Save effects; they do
  not claim user-configured backup/autosave compatibility.
- The two pane-local status lines match Micro's metadata order and formatting;
  the shortcut menu has two rows, comma separators, exact Micro key-menu order,
  and `^W Switch Panes` before it. The pane indicator and product/version label
  occupy the specified right edges without hiding file metadata.
- The product/version label and `--version` read the same application version
  value, currently `v0.0.1`.
- With Micro's default configuration, startup, editing, and quit follow the
  pinned Micro persistence behavior; tests cover any default backup/autosave
  effects identified in TASK-001 and verify explicit saves write only the
  focused pane's content.
- `just format`, `just lint`, `just test`, `just test-pty`, `just build`, and
  `just vet` pass.

Verification:

- `just format`
- `just lint`
- `just test`
- `just test-pty`
- `just build`
- `just vet`

Handoff:

- Record Micro persistence APIs reused, path/metadata behavior, tested prompts,
  two-pane adaptations, and any remaining differences from Micro with their
  evidence and owner disposition.

Implementation handoff:

- The CLI now accepts zero to two file operands and rejects a third before
  terminal initialization. One operand opens in the left pane; two open
  left-to-right. Empty and replacement documents use Micro `BTDefault` buffers.
- Startup and interactive Open use Micro's file loading, path metadata, file
  type, encoding, line-ending detection, and prompt flow. Save and Save As use
  Micro's buffer save path. Ctrl+T replaces the focused pane with a new unnamed
  Micro buffer and does not create a tab.
- Micro normally reuses the shared buffer for a path already present in
  `OpenBuffers`. That conflicts with mdiff's requirement for independent panes,
  so `NewBufferFromFileUnshared` follows the same Micro loading path while
  bypassing that reuse. The local Micro `OpenCmd` uses this constructor too.
- Ctrl+Q follows Micro's modified-buffer workflow for both panes: named buffers
  use the configured autosave behavior when enabled; otherwise each dirty pane
  receives a Save/Discard/Cancel decision. Save decisions are collected before
  any write occurs, so canceling a later pane cannot persist an earlier pane. An
  active Micro prompt receives Ctrl+Q through Micro's prompt handler, where it
  cancels that prompt instead of exiting mdiff.
- Every pane-level save path checks Micro's per-buffer `reload` setting before
  saving an externally changed file, including Ctrl+S, quit/autosave, dirty
  Open, and New Tab replacement. The default prompt reloads the on-disk version
  on Yes and resumes the requested callback; No saves the in-memory version;
  Escape cancels the pending save and callback. This also protects independent
  buffers that refer to the same path. Regression tests cover same-path quit,
  dirty replacement, and changes made by another process.
- Simulation and PTY tests cover startup argument mapping, same-path
  independence, focused-pane Open/Save, Save As, dirty Open and New Tab Save /
  Discard / Cancel, deterministic write failures, failed prompted-save
  preservation, external-change save conflicts across Ctrl+S, quit/autosave,
  Open, and New Tab, two-pane quit decisions, unnamed-buffer quit Save / Discard
  / Cancel, active-prompt Ctrl+Q, and terminal restoration. TASK-011 does not
  claim compatibility with user-configured backup/autosave settings beyond
  autosave-on-quit; TASK-006 owns general configuration compatibility.
- Verification passed: `just format`, `PATH=/home/nevd/go/bin:$PATH just lint`,
  `just test`, `just test-pty`, `just build`, and `just vet`. Focused checks
  also passed for the quit and dirty replacement workflows. The explicit PATH is
  needed because this environment installs `golangci-lint` under the Go bin
  directory, which is not on the shell's default PATH.

### TASK-006 - Load Micro configuration safely

State: REVIEW

Goal:

- Respect the user's existing Micro settings, keybindings, colorschemes, and
  syntax definitions in both panes. Keep Micro configuration read-only; TASK-011
  separately enables explicit document Open and Save actions.

Dependencies:

- TASK-005 is `COMPLETED` so safe editing actions and action filtering are
  defined.
- TASK-011 is `COMPLETED` so configuration behavior is tested with file-backed
  panes and per-pane status metadata.
- Use the configuration path and read-only initialization patches recorded in
  [`micro_integration_prototype.md`](micro_integration_prototype.md).

Scope:

- Work in `internal/micro/config`, `internal/micro/runtime`, and
  `internal/prototype`; update the user-facing support table in
  `design_docs/micro_diff_editor.md` and concise usage documentation.
- Extend TASK-011's file-backed pane fixtures to verify user-configured
  persistence settings, including backup/autosave behavior, once Micro settings
  are loaded. TASK-011 covers Micro defaults and explicit Open/Save behavior;
  these configuration-specific checks belong here to avoid a dependency cycle.
- Load the standard Micro configuration directory and formats without creating
  defaults or rewriting user files. Define precedence among Micro settings,
  mdiff-specific flags, and any mdiff-namespaced settings file.
- Apply supported buffer, rendering, mouse, paste, indentation, and color
  settings to both panes. Use Micro's effective defaults, including its default
  line-number behavior, and honor the user's `ruler`, `relativeruler`, and other
  supported settings when configured. Honor a synthetic filetype override and
  Micro syntax definitions without opening a file.
- Load supported `bindings.json` actions while reserving mdiff focus, quit, and
  diff-navigation controls. Reject unsafe or unavailable actions with actionable
  diagnostics.
- Resolve shortcut-bar labels from the effective post-configuration bindings;
  cover remapped, removed, reserved, and unavailable actions. This completes the
  built-in-only shortcut hints introduced in TASK-005 without moving the
  configuration dependency earlier.
- Resolve configured colorschemes and document fallback behavior for
  mdiff-specific diff styles missing from a user's scheme. Style the shortcut
  bar using Micro's status bar colors so its foreground and background match the
  user's active Micro colorscheme (including the dark background in the current
  scheme).
- Update the compatibility matrix and usage documentation with observed behavior
  rather than blanket claims of full Micro compatibility.

Out of scope:

- Loading or running Lua plugins; see TASK-007.
- Writing Micro defaults, mdiff settings, or buffer state to the user's config
  directory.

Acceptance criteria:

- Fixture-based tests cover absent, valid, and malformed settings, bindings,
  colorscheme, and syntax files. Valid user files remain byte-for-byte unchanged
  after startup, edits, and shutdown.
- Using the file-backed pane fixtures introduced by TASK-011, tests verify that
  user-configured backup/autosave behavior matches Micro and that startup/edit/
  quit creates only the persistence artifacts enabled by those settings.
- Tests prove supported settings and keybindings affect both panes and reserved
  or unsafe actions cannot invoke shell commands or escape mdiff's lifecycle.
  Document Open and Save behavior is covered by TASK-011.
- Tests verify both panes match Micro's effective default line-number behavior
  and that configured `ruler` and `relativeruler` options are applied
  consistently to each pane.
- Fixture tests verify every supported Micro setting affects both panes, while
  mdiff-owned constraints such as exactly two panes and the shared diff viewport
  remain enforced.
- Tests verify `--diff-debounce` overrides `diff-debounce-ms` from the
  mdiff-namespaced settings file and that invalid values are rejected before
  terminal raw mode.
- Shortcut-bar tests prove labels follow user remaps and omit removed or
  reserved actions. TASK-011 adds Open/Save hints and tests their active-pane
  behavior; the existing `Ctrl+T` action replaces the focused document to keep
  the two-pane limit.
- Color rendering tests verify the shortcut bar uses the active Micro
  colorscheme's status bar foreground and background, with the documented
  fallback when those styles are unavailable.
- Syntax highlighting works for explicitly identified synthetic buffer types
  without filesystem detection. Missing/invalid schemes produce the documented
  fallback or actionable error without losing terminal restoration.
- A compatibility matrix labels each tested feature as supported, partial, or
  unsupported and names the reason for each limitation.
- `just format`, `just lint`, `just test`, `just test-pty`, and `just build`
  pass.

Verification:

- `just format`
- `just lint`
- `just test`
- `just test-pty`
- `just build`

Handoff:

- Configuration lookup follows `--config-dir`, `MICRO_CONFIG_HOME`,
  `$XDG_CONFIG_HOME/micro`, then `~/.config/micro`; user runtime assets override
  embedded assets by name. The command-line debounce overrides
  `mdiff.diff-debounce-ms` in `settings.json`.
- Startup does not create the Micro config directory or write source config,
  settings defaults, or buffer-state files. Explicit document saves and
  configured backups/autosave remain enabled.
- Both panes receive Micro's parsed options and synthetic filetype settings.
  Configured bindings are loaded with mdiff lifecycle controls reserved;
  unsupported bindings and malformed syntax are reported without blocking
  startup. Shortcut labels follow supported remaps and the menu uses the active
  colorscheme's statusline style.
- Tests cover valid, missing, and malformed configuration, unchanged user files,
  synthetic syntax selection, remapped/removed bindings, color fallback,
  configured backup behavior, and the existing autosave quit behavior.
- Verified: `just format`, `just lint`, `just test`, `just test-pty`, and
  `just build` pass.

### TASK-007 - Complete Micro plugin compatibility within the two-pane model

State: NEW

Goal:

- Reuse Micro's plugin startup and loading behavior by default, adapting only
  plugin features that require more than mdiff's two panes or independent
  vertical viewports.

Dependencies:

- TASK-006 is `COMPLETED` and ordinary config loading/action filtering is
  covered.
- TASK-011 is `COMPLETED` so plugin lifecycle tests cover file-backed panes.
- Plugin behavior follows Micro's configured startup behavior; file and process
  side effects are permitted as they are in Micro.

Scope:

- Work in `cmd/mdiff`, `internal/micro/config`, `internal/prototype`, and the
  compatibility documentation; make no changes to the upstream Micro checkout.
- Load configured plugins using Micro's normal paths, settings, and lifecycle;
  do not require an mdiff-only opt-in.
- Expose both panes through Micro-compatible hooks and maximize reuse of Micro's
  plugin APIs. Preserve ordinary plugin file/process behavior.
- Adapt plugin actions to the paired panes and shared viewport. Only features
  that require creating more than two panes or breaking diff alignment may be
  limited; document each such case.
- Test representative plugins for load order, pane-local edits, file/process
  behavior, and adaptations to the two-pane model.

Out of scope:

- A sandbox or capability security model not present in Micro.
- Plugin features that require a third pane or independent pane scrolling.

Acceptance criteria:

- Tests prove configured plugins load using Micro's normal startup behavior.
- A compatible test plugin can observe and edit the intended focused pane
  through supported hooks; its changes advance the correct version and update
  the diff.
- Plugin features that require a third pane or independent scroll state are
  adapted or clearly documented. Tests verify normal Micro plugin file/process
  effects are not blocked by mdiff-only restrictions.
- Documentation states the tested compatibility set and any two-pane-specific
  limitations; it does not claim a sandbox.
- `just format`, `just lint`, `just test`, `just test-pty`, and `just build`
  pass.

Verification:

- `just format`
- `just lint`
- `just test`
- `just test-pty`
- `just build`

Handoff:

- Record plugins tested, hooks supported, and two-pane-specific adaptations.
  Identify representative fixtures by category: pane-local editing, file
  operations, tabs/splits, and independent scroll behavior.

### TASK-008 - Harden shared viewport and terminal interaction

State: NEW

Goal:

- Make cursor movement, scrolling, diff navigation, mouse input, wrapping,
  horizontal scrolling, and resize stable against the final diff/layout model.

Dependencies:

- TASK-003 and TASK-004 are `COMPLETED` so line and character diffs are
  versioned and installed atomically.
- TASK-005 is `COMPLETED` so supported editing actions and cursor behavior are
  stable.
- TASK-006 is `COMPLETED` so configured wrapping, pane, and mouse settings are
  known.

Scope:

- Work in `internal/viewport` and `internal/prototype`, including simulation and
  PTY tests; run supported terminal interaction checks on Linux and macOS CI.
- Keep one authoritative expanded visual-row offset that includes unmatched line
  spacers and unequal wrap continuation rows. Route wheel and keyboard scrolling
  from either pane to that offset; keep horizontal offsets pane-local.
- Complete next/previous difference navigation, click-to-focus and click-to-
  cursor mapping, deterministic clicks on virtual spacers, and cursor visibility
  after all edit/navigation/resize paths.
- Preserve top-row/subrow context through diff replacement, hunk removal,
  changed-line edits, terminal resize, gutter changes, and wrap option changes.
- Define deterministic behavior for softwrap on/off, narrow panes, terminal
  sizes below the preferred minimum, horizontal clipping, and cursor movement to
  a spacer counterpart.
- Add property-based tests for logical-to-visual mapping and interaction tests
  for repeated/ambiguous lines. Keep renderer updates atomic across both panes.

Out of scope:

- Incremental height data structures unless TASK-009 measurements require them.
- Animated diff transitions or independent pane vertical scrolling.

Acceptance criteria:

- For generated alignments and wrap heights, every real logical line maps to
  exactly one aligned record; logical line/subrow and visual row conversions are
  inverse where defined; both panes use the same visual coordinate.
- Simulation tests cover cursor movement and selections through insertions,
  deletions, replacements, spacer clicks, both scroll directions from both
  panes, diff jumps, softwrap on/off, unequal widths, and repeated-line anchor
  ambiguity.
- Resize and edits above/at/below the viewport preserve the specified anchor,
  keep the focused cursor visible, and produce no paired-line drift or one-frame
  mismatch between panes.
- PTY tests exercise focus, keyboard scrolling, resize, mouse wheel when
  supported, normal quit, interrupt, and terminal-mode restoration. Unsupported
  terminal protocols use an explicit fallback.
- `just format`, `just lint`, `just test`, `just test-pty`, and `just build`
  pass on Linux; supported simulation/build tests pass on macOS CI.

Verification:

- `just format`
- `just lint`
- `just test`
- `just test-pty`
- `just build`

Handoff:

- Record the final scroll/cursor rules, property generators and seeds, tested
  terminal capabilities, and any behavior that remains terminal-dependent.

### TASK-009 - Meet large-input performance and resource targets

State: NEW

Goal:

- Validate the proposed large-input latency and memory targets, then optimize
  only measured bottlenecks while keeping input responsive and comparison
  resource use bounded.

Dependencies:

- TASK-004 is `COMPLETED` with bounded debounced comparison.
- TASK-008 is `COMPLETED` with final rendering and visual-height behavior.

Scope:

- Add Go benchmarks alongside `internal/compare`, `internal/viewport`, and
  `internal/prototype` workloads, and expose them through `justfile`.
- Add reproducible Go benchmarks for 10,000 lines, 100,000 lines / 10 MiB, 1 MiB
  paste, near-equal single-line changes, large replacements, repeated lines,
  all-different input, long lines, and unequal wrap widths. Pin adversarial
  fixtures: (a) 100,000 copies of `repeat` versus 50,000 copies, one `changed`
  line, then 50,000 copies; (b) 50,000 unique fixed-width `L%08d:%016x` lines
  where the hex value is the next `Uint64` from Go
  `math/rand.NewSource(20261011)`, compared with the same generated records in
  reverse order; and (c) one 1 MiB ASCII line with a single middle-byte change.
  Record generator version and exact byte/line counts in benchmark output.
- Define the RSS fixture `rss-10mib-100k-balanced-near-equal-v1` exactly: each
  pane has 50,000 LF-terminated ASCII lines. In each pane, lines 0–42,879 have
  104 payload bytes and lines 42,880–49,999 have 103 payload bytes, for exactly
  5 MiB per pane and 10 MiB total including LF bytes. A line's payload starts
  with `L%05d:` for its zero-based line index; fill the remaining payload bytes
  with `byte('a' + (lineIndex+payloadOffset)%26)`, where payloadOffset is
  zero-based after the prefix. The right pane initially matches the left, then
  on lines divisible by 1,000 changes the first payload byte after the prefix to
  the next lowercase alphabet character, wrapping `z` to `a`. This yields 50
  one-byte changed lines, no insertions or deletions, identical line counts and
  line lengths, and an exact total input size of 10,485,760 bytes. Keep the
  fixture generator deterministic and version it with this fixture name.
- Add a documented `just bench` recipe that runs the relevant benchmarks with
  repeatable flags and keeps benchmark output outside the source tree by
  default.
- Measure input-to-visible-text latency separately from settled diff latency,
  plus allocations and peak resident memory where the platform tooling permits.
- Implement a documented timeout/fallback for adversarial comparisons that
  returns a usable coarse changed region and status warning no later than 2 s
  after comparison starts instead of blocking the UI. Keep at most one active
  and one latest pending snapshot pair; timeouts must not strand unbounded
  workers or snapshots.
- Cache line display widths by content version, line, pane width, tab size, and
  wrap mode. Invalidate only affected lines on edit and all affected heights on
  resize or relevant configuration changes.
- Avoid repeated whole-document copies where safe immutable line snapshots are
  available. Do not introduce incremental diff or tree structures without
  benchmark evidence.

Out of scope:

- Changing the performance targets without recording reference machine,
  terminal, input shape, and measured evidence.
- Hiding lag by suppressing diff output or treating all large documents as
  unchanged.

Acceptance criteria:

- Benchmarks record machine/OS/Go version, input shapes, latency distributions,
  allocations, and memory observations in a reproducible report.
- On the reference profile of Ubuntu 24.04 Linux/amd64, 4 vCPUs, and 8 GiB RAM,
  with the exact Go 1.26.x patch version recorded, each adversarial comparison
  either completes or installs its coarse fallback within 2 s of worker start.
  Peak process RSS for `rss-10mib-100k-balanced-near-equal-v1` must remain at or
  below 512 MiB. The benchmark report includes its exact generated input
  parameters and measured RSS; these limits are regression gates, not claims
  about every user machine.
- Targets from [`micro_diff_editor.md`](micro_diff_editor.md) are met, or the
  plan is updated with measured rationale and an explicit owner decision before
  accepting a revised target.
- A 1 MiB paste becomes visible within 250 ms on the documented reference setup;
  event handling remains responsive while diff work runs.
- The comparison queue remains bounded under sustained edits; cancellation,
  timeout, and shutdown leave no worker or retained snapshot leak.
- Each adversarial input completes or triggers the documented coarse fallback
  within 2 s of worker start and still produces aligned, navigable output.
- `just format`, `just lint`, `just test`, `just test-race`, `just build`, and
  the benchmark command pass.

Verification:

- `just format`
- `just lint`
- `just test`
- `just test-race`
- `just build`
- `just bench`

Handoff:

- Record baseline and final results, any target changes, fallback thresholds,
  cache invalidation rules, and evidence for every optimization retained.

### TASK-010 - Complete release readiness and platform verification

State: NEW

Goal:

- Prepare a reviewable release candidate with accurate Micro-compatible user
  documentation, preserved licenses, verified per-pane file workflow, and
  repeatable Linux/macOS build and terminal checks.

Dependencies:

- TASK-003 through TASK-009 are `COMPLETED`.
- TASK-011 is `COMPLETED` so release documentation and CLI checks cover file
  arguments and per-pane persistence.
- Existing Go scaffold, Just recipes, CI workflows, and Jujutsu conventions
  remain the project's supported development path.

Scope:

- Update `README.md`, `docs/`, `design_docs/`, `justfile`, and
  `.github/workflows/ci.yml` as needed; preserve upstream files and notices
  under `internal/micro/`.
- Document launch, focus, editing, paste, per-pane file open/save, diff
  navigation, scrolling, wrapping, resize, config compatibility, plugin loading,
  and quit in the user-facing README/help.
- Verify all Micro and third-party license notices, vendored-source pin and
  local patch inventory, and the documented upstream refresh process.
- Review CLI behavior for supported file arguments and flags; invalid inputs
  must fail before entering raw terminal mode.
- Exercise release build reproducibility and CI on supported Linux/macOS
  targets, including PTY gates where available, dependency audit, and terminal
  cleanup on normal quit and catchable interruption.
- Check byte-for-byte reproducibility separately on each supported release
  runner: from the same clean revision, Go patch version, dependency sums,
  environment, and flags, build the `mdiff` executable twice into distinct
  temporary paths using
  `go build -buildvcs=false -trimpath -ldflags="-s -w" -o "$tmp/mdiff" ./cmd/mdiff`
  and require `cmp` equality. Run this on Ubuntu 24.04/Linux amd64 and macOS 15
  on the runner's native architecture; do not compare hashes across operating
  systems or architectures. Record command, toolchain, target, and SHA-256 of
  the verified artifact for each runner.
- Add a pinned/documented `just audit` command and CI job for Go dependency
  vulnerability/license checks, using a maintained Go-aware scanner and explicit
  tool version rather than fetching an unpinned latest binary.
- Use `jj` and `difftastic` for local change review where available; keep
  release artifacts and generated diagnostics out of the source change unless
  the project explicitly tracks them.

Out of scope:

- Claiming macOS/terminal compatibility that has not run in CI or a documented
  manual verification environment.

Acceptance criteria:

- User documentation matches the tested Micro compatibility matrix and clearly
  explains the two-pane model, per-pane filenames, and supported file actions.
- License and notice inventory matches the shipped Micro source and direct
  dependencies; upstream pin and patch refresh steps are reproducible.
- Linux and macOS CI build, lint, test, and release-build jobs pass; PTY checks
  pass on supported runners or have a documented platform-specific reason for
  being skipped.
- The two same-runner release builds are byte-identical for each supported
  target. The workflow records the target and verified SHA-256; a mismatch is a
  release blocker that must be diagnosed before acceptance.
- CLI validation, terminal restoration, normal exit, and interrupt after edits
  are verified. File writes occur through Micro-compatible explicit save and
  configured persistence behavior; configuration files are not rewritten.
- The complete `just check` gate passes on a clean working copy and the release
  notes list known limitations, including disabled-by-default trusted plugins.

Verification:

- `just format`
- `just check`
- `just audit`
- Linux/macOS CI workflow and dependency audit.
- The documented upstream source/license verification procedure.

Handoff:

- Record CI run links or job identifiers, release build hashes, license audit
  results, known compatibility limits, and final acceptance evidence. Rufus
  performs the release-readiness review and marks TASK-010 `COMPLETED` only
  after all criteria pass.
