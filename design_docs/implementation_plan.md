# Implementation Plan

TASK-001 through TASK-013 are complete. Earlier implementation and review
history is available in the repository history. TASK-013's approved handoff
remains below until its change is merged.

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

### TASK-013 - Complete the blocked-command audit

State: COMPLETED

Goal:

- Account for every registered Micro command denied by `safeCommand`, and
  restore any command that can preserve mdiff's fixed panes and shared viewport.

Dependencies:

- TASK-012 is `COMPLETED`, including its terminal ownership changes.

Scope:

- Audit the complete denied built-in list in `internal/prototype/app.go`:
  `vsplit`, `hsplit`, `tab`, `tabmove`, `tabswitch`, `term`, `eval`, `log`,
  `plugin`, `reload`, and `raw`. Record each command's actual behavior in the
  pinned Micro source and the specific mdiff invariant it would affect.
- Record that `raw` opens another tab, `log` and `plugin` open a log view, and
  bundled Micro's `eval` reports unsupported. Assess `reload` in detail for
  settings, bindings, plugins, colorscheme, buffer settings, the fixed panes,
  and the shared viewport. If it is safe, route it through Micro and refresh
  mdiff-owned state as needed; otherwise retain the guard and document the exact
  invariant it breaks.
- Include `mdiff.diff-debounce-ms` in the `reload` decision. If `reload` is
  allowed, a valid reloaded value takes effect immediately in the running diff
  coordinator. Restart an already pending debounce from the reload instant using
  the new delay; a CLI override still wins and leaves its timer unchanged. Use
  the startup resolver and its 40–180 ms rule before invoking Micro's reload.
  Without a CLI override, an invalid value must produce a visible error and
  leave the previous effective delay, pending timer, and Micro settings intact.
  With a valid CLI override, the file value is ignored as at startup. If
  `reload` remains denied, document the mismatch between reloaded settings and
  the coordinator's startup delay as one concrete reason.
- Update command documentation, focused command-routing tests, the technical
  design, and release notes. Keep rejection messages visible for commands that
  remain unsupported.

Out of scope:

- Creating extra tabs, panes, log views, or independent vertical viewports.
- Replacing Micro's command registry or adding a second executable-level
  behavior matrix.

Acceptance criteria:

- Every denied registered built-in command has a documented source-backed reason
  or a tested adapter that preserves exactly two panes and one shared vertical
  viewport. The documented list matches the actual guard.
- `reload` has an explicit decision. If allowed, a regression covers invoking it
  from the command bar after editing both panes and verifies bindings, settings,
  colors, focus, buffer identity, diff alignment, and subsequent input. If
  denied, the documented reason identifies the concrete invariant or lifecycle
  failure demonstrated by source inspection or a focused test. The decision also
  states whether the debounce setting is live or why `reload` remains blocked.
- If `reload` is allowed, deterministic coordinator tests verify actual
  scheduling after a valid changed debounce, an invalid changed value without a
  CLI override, and a CLI override, with a debounce already pending in each
  case. Invalid input leaves the prior effective delay and timer schedule in
  place; a valid reload restarts a pending timer from the reload instant.
- Allowed commands continue to reach Micro in the focused pane; blocked commands
  produce a visible explanation and do not alter pane state.
- The patch version is bumped once in this task's single Jujutsu change, and the
  release note and exact-version assertions agree.

Verification:

- `just format`
- `just lint`
- `just test`
- `just build`
- `just test-pty` if terminal-facing command behavior changes.

Handoff:

- Audited all eleven formerly denied registered built-ins against the pinned
  Micro handlers. `eval` reaches Micro's `Eval unsupported` message. The other
  ten commands remain visibly denied because they create or move views, replace
  a pane, require a coordinated runtime reload, or can repeat a plugin
  activation callback after resize. The technical design records each source
  path and invariant.
- Kept `reload` blocked: Micro reapplies settings, bindings, plugins,
  colorscheme, and buffer settings without rebuilding mdiff's aligned viewport;
  `mdiff.diff-debounce-ms` is resolved at startup and the running coordinator
  retains that delay and its pending timer. README command guidance and focused
  routing tests cover the decision and unchanged pane state.
- Updated the patch version and release notes once in this JJ change. Removed
  TASK-012's approved specification and review from active documents; both
  remain in repository history.
- Verification passed: `just format`, `just lint`, `just test`, `just build`,
  and `just test-pty` on Linux. macOS behavior remains for CI or independent
  review.
- Addressed review pass 1 by restoring the `tabswitch` guard. A resize replaces
  mdiff's Micro tab without marking that tab active, so Micro's `tabswitch 1`
  could repeat a plugin `onSetActive` callback. The command documentation and
  routing regression now cover the guarded behavior after resize; `eval` remains
  routed to Micro.
- Independent review pass 2 found no open findings and approved completion.

### TASK-014 - Share Micro's soft-wrap source-position mapping

State: NEW

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
