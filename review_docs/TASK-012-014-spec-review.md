# TASK-012 through TASK-014 specification review — 2026-10-16

**Pass:** 2\
**Decision:** Ready for implementation; no open specification findings\
**Scope:** The three `NEW` tasks in
[`implementation_plan.md`](../design_docs/implementation_plan.md), checked
against the current Micro integration and the first usable release objective.
These tasks are follow-up work after that release. This is a specification
review; no implementation or task state was changed.

## Pass 1 findings (resolved in pass 2)

### 1. High — TASK-012 does not name the screen, shell, and signal changes needed for a safe handoff

TASK-012 names `internal/micro/action` and `internal/prototype` as its
adaptation points, yet the handoff itself is inside Micro's
[`shell.RunInteractiveShell`](../internal/micro/shell/shell.go) and
[`screen.TempFini`/`TempStart`](../internal/micro/screen/screen.go). `TempStart`
discards the error returned by `screen.Init`. Micro's shell routine also resets
and re-registers `SIGINT`, while mdiff owns a `signal.NotifyContext` for
`SIGINT` and `SIGTERM`. mdiff's event goroutine polls the screen captured at
startup, which is replaced by a terminal restart
([`RunWithFiles`](../internal/prototype/app.go)). The current named adaptation
points alone cannot demonstrate the task's promised handling of restart failure,
signal ownership, or a second handoff.

**Required specification change:** Include the Micro `screen` and `shell`
lifecycle APIs in scope, or explicitly define an mdiff-owned replacement for
their handoff. Require an error-returning restart path, one owner for signal
registration, and a reader that is stopped before release and attached to the
new screen after restart. Acceptance tests must cover a command interrupted by
Ctrl+C, two consecutive shell handoffs, a suspend/resume cycle followed by
another input event, and a forced restart failure that exits with the terminal
restored rather than continuing on the old screen. Specify how a pending diff
result is installed after resume without painting a released screen.

### 2. Medium — TASK-012 can return before the user sees shell output

Micro's `ShellMode` calls `RunInteractiveShell` with `wait=true`; after a
successful command it calls `screen.TermMessage("")` to wait for Enter before
returning. mdiff installs a global `screen.MessageHandler` that redirects every
`TermMessage` into the editor status area, so that call does not wait or keep
the command's output visible. The task says to preserve Micro's shell return
behavior, but its acceptance tests only require that a command returns to the
editor.

**Required specification change:** State how the handoff temporarily restores
Micro's terminal wait prompt, or define an equivalent explicit Enter-to-return
step. A PTY test should run a command that prints a unique marker, verify the
marker remains visible until Enter, and then verify editing works after the
screen redraw. Cover the failed-command path separately so its error remains
visible and the terminal still returns to mdiff.

### 3. High — TASK-014 mixes rune offsets with Micro character offsets

TASK-014 requests a location for every **rune** boundary and asks tests to
compare those locations directly with Micro's position behavior. The current
mdiff annotation indexes `[]rune` and changed spans by rune offset
([`render.go`](../internal/prototype/render.go)). Micro's `buffer.Loc.X` instead
indexes decoded characters: `util.DecodeCharacter` consumes a base rune with
following combining marks, and `VLocFromLoc` accepts that character offset
([`softwrap.go`](../internal/micro/display/softwrap.go)). For `e` followed by a
combining accent, there are two rune offsets but one Micro character offset.
Without a defined translation, an API cannot both return every rune boundary and
use the same index directly as `VLocFromLoc`.

**Required specification change:** Define the new API's index domain and the
translation from rune spans to Micro character positions. Specify where a
combining mark's zero-width boundary maps, including at a visual wrap and end of
line. Add decomposed Unicode cases to the acceptance tests and compare only
equivalent positions. Keep the existing changed-character and selection tests as
renderer regressions.

### 4. Medium — TASK-014 measures only the batch path, not Micro's cursor path

TASK-014 calls for one shared calculation, but says Micro's existing
single-position conversion should use it only "where practical." The existing
`VLocFromLoc` caches individual locations and can stop after reaching the
requested position. Replacing each query with a newly allocated whole-line map
would retain linear batch production but add line-sized work and memory to
ordinary cursor motion. Conversely, leaving the old algorithm separate would not
achieve the stated one-calculation goal.

**Required specification change:** State whether the shared implementation is a
scanner with single-position and all-boundaries callers, or a cached per-line
map with explicit invalidation on text, width, and wrap-setting changes. Measure
both all-boundaries production and repeated cursor-position queries on a long
wrapped line. Acceptance should reject a per-query full-line allocation or a
regression from linear total work to repeated prefix scans.

### 5. Medium — TASK-013 does not decide what `reload` does to mdiff's debounce setting

Micro's `ReloadCmd` reloads settings, bindings, plugins, colors, and buffer
settings. mdiff reads `mdiff.diff-debounce-ms` once during `NewWithFiles` and
constructs its coordinator with that value; the command-line override takes
precedence ([`app.go`](../internal/prototype/app.go)). TASK-013 asks for a
`reload` decision and tests "settings," but those tests could pass while the
settings file shows a new debounce value and the running diff worker keeps the
old one. An invalid reloaded value may likewise escape the startup validation
path.

**Required specification change:** If `reload` is allowed, specify whether
`mdiff.diff-debounce-ms` applies immediately or is explicitly a startup-only
setting. Test a changed valid value, an invalid value, and CLI-override
precedence against actual diff scheduling, including a pending debounce. If
`reload` is denied, document this coordinator mismatch among the concrete
reasons rather than merely saying it may affect settings.

### 6. Low — TASK-013 requires a version bump even if the audit changes documentation only

The plan requires exactly one patch-version bump in **every** task, including
TASK-013. Its accepted outcome may be a source-backed decision to retain all
guards, with changes only to command documentation and tests. The
[`development policy`](../docs/development.md) says documentation, tests, and
formatting alone do not require a bump. Forcing one in that outcome adds a
release version without changing shipped behavior.

**Required specification change:** Make TASK-013's version criterion follow the
material-code rule: bump once if the task changes shipped code or Micro
integration; otherwise record the audit without a version bump. Keep the
one-change rule and release-note requirement for a material change.

## Pass 1 task readiness (superseded)

| Task     | Decision               | Reason                                                                                                                                          |
| -------- | ---------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| TASK-012 | Revise before starting | Specify restart-error propagation, signal and reader ownership, and Micro's wait-for-Enter behavior.                                            |
| TASK-013 | Revise before starting | Decide the mdiff debounce contract for `reload` and align versioning with the documented policy. Its dependency on TASK-012 remains as written. |
| TASK-014 | Revise before starting | Define rune-versus-character indexing and protect both batch and cursor-path performance.                                                       |

These findings do not block the first usable release: all three tasks are
already listed as follow-up work. They prevent the task specs from being
self-contained enough for Igor to implement and Rufus to approve without making
unstated behavior decisions. The plan still labels TASK-012 `NEW`, which
normally means ready to start. Resolve its specification findings and reconcile
that state before Igor selects it.

## Verification

Read the active plan, agent workflow, technical design, development policy, and
the relevant Micro and mdiff source. Confirmed that `setConfigDir` now creates
the default config directory and has a regression test, so no config-directory
task is needed. No Go tests or build were run for this documentation-only
specification review. The pinned `mdformat --check` passed on this document.

## Pass 2 re-review

Igor's revised plan resolves all six findings. The earlier findings and task
readiness table above remain as the review history; the current decision is
below.

| Finding                              | Status   | Revised plan evidence                                                                                                                                                                                                                                                                    |
| ------------------------------------ | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1. Screen, shell, and signal handoff | Resolved | TASK-012 now names the Micro screen and shell APIs, one mdiff lifecycle owner, restart errors, a stopped and restarted event reader, repeated handoffs, and pending diff results. Its acceptance tests exercise interruption, suspend/resume, consecutive handoffs, and restart failure. |
| 2. Shell output and Enter prompt     | Resolved | TASK-012 requires a terminal wait prompt while output remains visible, a separate failed-command path, and PTY coverage of both.                                                                                                                                                         |
| 3. Rune and character offsets        | Resolved | TASK-014 defines the two index domains, a rune-to-character boundary map, combining-mark behavior at wrap and end of line, and translation for selections and wrap height.                                                                                                               |
| 4. Cursor-path performance           | Resolved | TASK-014 requires one Micro-owned scanner with a bounded single-position path and a one-pass batch path. Both are benchmarked.                                                                                                                                                           |
| 5. Reloaded debounce                 | Resolved | TASK-013 defines valid, invalid, and CLI-override behavior for the running coordinator and pending timer. Its acceptance tests verify scheduling in each case if `reload` is allowed.                                                                                                    |
| 6. Documentation-only version bump   | Resolved | The plan's preamble now records the owner's explicit exception requiring one patch bump for each task, including a documentation-only TASK-013 outcome.                                                                                                                                  |

**Current task readiness:** TASK-012 is ready to start. TASK-013 and TASK-014
are ready in sequence after their stated dependencies are complete. The tasks
remain follow-up work after the first usable release.

The reload contract is conditional on TASK-013's source-backed command decision.
If `reload` is allowed, its invalid-value guarantee requires preflight of the
on-disk value before Micro's reload mutates settings; Micro's current
`config.ReadSettings` replaces its parsed-settings state as it reads. This is an
implementation constraint already covered by the revised task's required
outcome, not an open specification finding. Preserve Micro ownership of the
settings parser when implementing that branch.

This pass re-read the revised task plan and the relevant Micro settings, reload,
screen, shell, and suspend paths. No implementation code was changed and no Go
tests were run for this specification review.
