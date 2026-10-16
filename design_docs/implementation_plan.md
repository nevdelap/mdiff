# Implementation Plan

TASK-001 through TASK-011 are complete. Their implementation and review history
is available in the repository history; no completed task specifications remain
in the active plan.

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

### TASK-012 - Restore interactive shell and suspend actions

State: COMPLETED

Goal:

- Let Micro's `ShellMode` and `Suspend` actions hand terminal control away and
  return to the same editable, diff-aligned two-pane session.

Dependencies:

- TASK-001 through TASK-011 are `COMPLETED`.
- Start this task before TASK-013 and TASK-014.

Scope:

- Adapt the action filter and routing in `internal/micro/action` and
  `internal/prototype`, together with the lifecycle APIs in
  `internal/micro/screen` and `internal/micro/shell`, so the default `ShellMode`
  binding and configured `ShellMode` and `Suspend` bindings work on Linux and
  macOS terminals. Keep the Micro shell prompt and command semantics.
- Put terminal and process-signal ownership in one mdiff lifecycle controller.
  Micro's shell path must not independently reset or register `SIGINT` while
  mdiff's `signal.NotifyContext` is active. During an interactive child command,
  Ctrl+C must interrupt the child without quitting mdiff; after return, mdiff
  must resume its normal `SIGINT` and `SIGTERM` shutdown handling.
- Stop and await the mdiff event reader before releasing its screen, then start
  a reader attached to the new screen after resume. Make the screen restart API
  return initialization errors; on failure, restore terminal modes and exit with
  an error instead of polling or drawing through the old screen. The same
  lifecycle must support repeated shell handoffs and suspend/resume.
- During handoff, keep diff workers on immutable snapshots but defer app-side
  result installation and all drawing. After resume, process pending results
  through the existing version check and viewport-anchor path, then resize and
  redraw both panes before accepting new input.
- Temporarily restore Micro's terminal wait prompt, or show an equivalent
  explicit Enter-to-return prompt, while shell output is visible. Restore
  mdiff's status-message handler when the two-pane screen resumes; report a
  failed command visibly without losing terminal ownership.
- Keep the two existing buffers, focus, selections, dirty state, and pending
  diff work intact. Update the README, technical design, and release notes to
  describe supported behavior and any platform limit.

Out of scope:

- Additional terminal panes, tabs, or a separate shell view.
- A second executable-level Micro versus mdiff behavior matrix.

Acceptance criteria:

- Default and remapped `ShellMode` bindings open Micro's prompt; a canceled
  prompt leaves the editor usable. A PTY command prints a unique marker that
  remains visible until Enter; only then does the two-pane screen redraw and
  accept edits. A failed command has a separate test for visible error reporting
  and a usable return path.
- `Suspend` releases terminal control, and continuing the process restores the
  two-pane screen and functional editing, pane switching, diff alignment, and
  quit behavior. No reader polls a released screen, and resumed input comes from
  the restarted screen.
- PTY tests cover Ctrl+C interrupting a command without exiting mdiff, two
  consecutive shell handoffs, suspend/resume followed by an input event, and
  terminal restoration on exit. A controlled restart-initialization failure test
  verifies error propagation, terminal-mode restoration, and no use of the stale
  screen. In-process tests cover routing and state preservation.
- A diff result completed during handoff is installed or discarded according to
  its buffer versions before the first resumed draw; no released screen is
  painted. Tests run on supported platforms or explicitly skip only when an
  operating-system capability is absent.
- The patch version is bumped once in this task's single Jujutsu change, and the
  release note and exact-version assertions agree.

Verification:

- `just format`
- `just lint`
- `just test`
- `just test-pty`
- `just build`
- Run and record a manual shell and suspend/resume check on an available real
  terminal; record any supported platform that could not be checked locally.

Handoff:

- Restored the default and configured Micro shell action and configured suspend
  action. `internal/prototype` owns the signal listener, screen reader, terminal
  release/restart, and pending diff installation. The Micro screen API now
  returns restart errors; the shell API can run an interactive command under
  mdiff's terminal owner and hold its output until Enter.
- Added PTY coverage for two shell handoffs, output acknowledgement, command
  failure, Ctrl+C interruption, suspend/resume followed by input, and normal
  terminal restoration. In-process tests cover a completed diff during handoff
  and a forced screen restart failure without a stale reader or screen.
- Addressed the implementation review findings: failed tcell initialization now
  clears the partial screen without finalizing it; both shell routes use one
  Micro-owned command executor; and the tests distinguish command output from
  echoed input, verify two-pane state and alignment after resume, discard a
  stale diff result, and check terminal modes after a real restart failure. The
  follow-up review found an unnecessary handoff for invalid shell input; Micro
  now validates it before terminal release, with app-route tests for an empty
  command and malformed quoting.
- Verification passed: `just format`, `just lint`, `just test`, `just test-pty`,
  `just build`, and `just test-race`. A manual Linux terminal session ran
  `Ctrl+B`, displayed command output until Enter, resumed the editor, suspended
  through a configured `Ctrl+R` binding, resumed with `SIGCONT`, accepted an
  edit, and exited through the dirty-buffer prompt. macOS terminal behavior
  could not be checked locally and remains for CI or independent review.
- The single task change increments the patch version once and updates the
  release notes. Independent review pass 3 found no open findings and approved
  the implementation for completion.

### TASK-013 - Complete the blocked-command audit

State: NEW

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
