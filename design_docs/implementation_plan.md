# Implementation Plan

TASK-001 through TASK-013 are complete. Earlier implementation and review
history is available in the repository history.

The tasks below are ordered from most to least valuable and must be implemented
in that order. Complete each task in one Jujutsu change, with exactly one patch
version bump in `internal/version/version.go` and a corresponding entry in
`docs/release_notes.md`. Review fixes amend the same change and do not bump the
version again. The owner requires a bump for each task in this sequence,
including a documentation-only outcome for TASK-013; this is an explicit
exception to the general material-code rule in `docs/development.md`. The
product direction and durable architecture are in
[micro_diff_editor.md](micro_diff_editor.md), and the task process is in
[agent_workflow.md](agent_workflow.md).

## Active tasks

### TASK-015 - Restore bundled Micro behavior parity

State: COMPLETED — approved in pass 8 of
[`micro-behavior-parity-review.md`](../review_docs/micro-behavior-parity-review.md)

Goal:

- Resolve every required finding in the cross-cutting bundled Micro behavior
  parity review while retaining mdiff's fixed paired-pane and shared-diff view.

Dependencies:

- TASK-014 is merged and the review document is included with this task.

Scope:

- Give bound `QuitAll` one discard-all confirmation. Preserve the action-chain
  fallback when diff navigation has no target, and navigate from the focused
  cursor's aligned row.
- Adapt Micro's cursor and selection page actions to the shared viewport.
- Parse commands with Micro's shell quoting and exact command-name behavior.
- Use Micro help buffers and rendering in the overlay, including multiple
  requested help topics and configured help actions.
- Match Micro `SaveAll` calls to `Buffer.Save` for both documents.
- Dispatch wheel bindings to the pane under the pointer without changing focus.
- Honor and compose Micro scrollbars against the shared viewport. Keep mdiff's
  diff gutter as the replacement for Micro's VCS diff gutter.
- Route plugin reload through mdiff's guarded reload policy.
- Add focused regressions for every reviewed behavior and update the release
  note and patch version exactly once in this change.

Acceptance criteria:

- All nine findings in the parity review are resolved and covered by permanent
  regressions against the bundled Micro behavior.
- The two-pane layout and shared viewport remain intact.
- Format, lint, full tests, and build pass.

### TASK-014 - Share Micro's soft-wrap source-position mapping

State: COMPLETED — approved in pass 2 of
[`TASK-014-implementation-review.md`](../review_docs/TASK-014-implementation-review.md)

Goal:

- Use one Micro-owned calculation for source-rune visual locations in the
  renderer while keeping long-line rendering linear.

Dependencies:

- TASK-013 is `COMPLETED` so command and settings behavior is stable.

Scope:

- Add one shared scanner in `internal/micro/display`, using Micro's
  `util.DecodeCharacter` units and its current tab width, word-wrap, soft-wrap,
  and window-width rules. Its single-position caller must stop at the requested
  `buffer.Loc.X` character offset without allocating a whole-line map; its batch
  caller must produce visual positions and a rune-to-character boundary map for
  a whole line in one pass. Make `VLocFromLoc` and the new batch API use that
  scanner, retaining bounded caching for cursor lookups.
- Define the batch API's indices explicitly: Micro positions use decoded
  character offsets; comparison spans and `renderedCell.sourceRune` use Go rune
  offsets. The adapter maps each rune span to the character cluster it touches.
  For a base rune followed by combining marks, the base starts at the
  character's visual start, and every boundary after that base and through its
  marks maps to the character's visual end. Combining marks have zero separate
  cell width even when the character end is at the next visual row or end of
  line. A change touching only a mark highlights its base character.
- Replace `sourceRuneLocations` in `internal/prototype/render.go` with the Micro
  API. Translate Micro selection endpoints and other `buffer.Loc.X` uses in
  rendering from character to rune offsets, including wrap-height lookup. Remove
  the duplicate width and wrap calculation and keep the rendering behavior for
  changed characters, selections, and virtual rows.
- Update the technical design and release notes to reflect the shared mapping.

Out of scope:

- Changing the diff algorithm, visual alignment model, or Micro's user-facing
  wrapping rules.

Acceptance criteria:

- The duplicate mapping calculation is gone from mdiff's renderer. The Micro
  batch API supplies rune-boundary positions and their character offsets,
  including end of line, in O(line length) time and space. A single-position
  query scans at most through its requested character and does not allocate a
  full-line map on each cursor movement.
- Tests compare only equivalent Micro character boundaries with `VLocFromLoc`.
  They cover decomposed Unicode and multiple combining marks, wide characters,
  tabs, whitespace, word wrap on and off, soft wrap on and off, narrow panes, a
  mark at a visual wrap, end of line, and changed text spanning a wrap. Existing
  changed-character, selection, Unicode, tab, soft-wrap, and aligned-row
  rendering regressions pass.
- Long wrapped-line benchmarks measure both batch production and repeated
  cursor-position queries at increasing offsets. They demonstrate linear batch
  work and rule out a per-query full-line allocation or repeated prefix scans
  when producing the batch. Record fixtures, allocations, and measurements in
  the task handoff.
- The patch version is bumped once in this task's single Jujutsu change, and the
  release note and exact-version assertions agree.

Verification:

- `just format`
- `just lint`
- `just test`
- `just build`
- Run the focused mapping and rendering tests and both long-line benchmarks;
  record their commands and results.

Handoff:

- Added one Micro-owned decoded-character scanner for cursor locations, line
  ends, and one-pass rune-boundary maps. The renderer uses the batch map for
  changed cells and selection offsets; wrap-height lookup uses Micro character
  units. Removed mdiff's duplicate source-position calculation.

- Added exact visual-position cases and regressions for combining marks,
  selection after a mark cluster, a mark-only change, and changed text crossing
  a visual wrap. The existing Unicode, tab, wrapping, selection, and aligned-row
  tests also pass.

- Benchmarked on Linux amd64, Ryzen 7 5700U, with a 40-column soft- and
  word-wrapped line of repeated `word<TAB>界e◌́ ` groups. The command was
  `go test ./internal/micro/display -run '^$' -bench 'Benchmark(RuneBoundariesLongLine|CursorPositionLongLine)$' -benchmem -benchtime=100ms -count=1`.
  At 512/1024/2048 groups (6144/12288/24576 bytes), batch scans took
  0.800/1.738/3.025 ms and 635/1618/2898 kB per operation, with 563/1083/2114
  allocations. Uncached cursor queries at increasing offsets took
  0.118/0.230/0.446 ms and 2.5/4.1/7.2 kB per operation, with 195/387/768
  allocations. Batch time grew approximately with line length; the cursor path
  allocated no full-line rune map.

- `just format`, `just lint`, `just test`, and `just build` passed on Linux.
  Focused display and renderer tests passed with
  `go test ./internal/micro/display ./internal/prototype -run 'Test(RuneBoundaries|SelectionAfterCombiningCluster|ChangedTextSpanningVisualWrap|MarkOnlyChangeHighlightsBaseCell)' -count=1`.
  The patch version and exact-version assertion changed once in this JJ change.

- Addressed implementation review pass 1 in the same change without another
  version bump. Public negative Micro locations again resolve to line start;
  private line-end scans retain a separate sentinel. Unwrapped annotation now
  maps only source characters intersecting the visible horizontal range and
  stops at its right edge, preserving absolute rune and character offsets.

- Added regressions for negative locations with soft wrap enabled and disabled,
  and a horizontally scrolled 10,001-character changed line. Added
  `BenchmarkRenderUnwrappedLongLineAnnotations`, which redraws two
  1,048,577-character unwrapped lines around their changed midpoint.
  `go test ./internal/prototype -run '^$' -bench '^BenchmarkRenderUnwrappedLongLineAnnotations$' -benchmem -benchtime=500ms -count=1`
  measured 182.5 ms/op, 10.8 MB/op and 3312 allocs/op on Linux amd64, Ryzen 7
  5700U. The benchmark includes the full renderer redraw; the visible source map
  itself is bounded by the pane width.

- After these fixes, `just format`, `just lint`, `just test`, and `just build`
  passed. The focused review regressions passed with this command:

  ```sh
  go test ./internal/micro/display ./internal/prototype -run 'Test(Negative|Visible|UnwrappedHorizontal|RuneBoundaries|SelectionAfter|ChangedTextSpanning|MarkOnlyChange)' -count=1
  ```
