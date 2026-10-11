# TASK-002 Review

**Task:** Prototype Micro integration and shared diff scrolling\
**Reviewer:** Rufus\
**Review pass:** 2\
**Decision:** APPROVED

## Findings

### 1. The real terminal never receives the active cursor position

**Severity:** High\
**Status:** Resolved in pass 2

`App.draw` calls `drawCursor` while the global `screen.Screen` still points to
the offscreen composition surface. It then switches `screen.Screen` back to the
terminal and copies only cell contents before calling `Show`. The cursor
position/visibility is not copied to the terminal screen, so the active cursor
is absent or stale in the real terminal. This violates the acceptance criteria
that cursor navigation and resize keep the active cursor visible.

**Required resolution:** Set or hide the cursor on `a.terminal` using the same
mapped coordinates after restoring the terminal screen, and add a regression
test that inspects the terminal screen cursor after `draw` for visible, hidden,
and resize cases.

Evidence: [`app.go`](/home/nevd/mdiff/internal/prototype/app.go:584) and
[`app.go`](/home/nevd/mdiff/internal/prototype/app.go:598).

Resolution: `App.draw` now computes the shared cursor location and applies it to
the real terminal after cell composition. The simulation test checks visible and
hidden cursor states and the cursor after resize; the PTY test checks a visible
cursor after resize.

### 2. PTY checks do not prove the required interactions and use fixed sleeps

**Severity:** Medium\
**Status:** Resolved in pass 2

The PTY smoke test waits fixed 250/80/80/100 ms intervals around startup, paste,
and resize. It sends Ctrl-Up after an F6 sequence and Ctrl-Down after a second
F6 sequence, but only checks that text markers appeared somewhere in the
terminal transcript. It does not assert that scrolling occurred from each pane,
that resize retained the viewport/cursor, or that the interrupt path left the
config directory free of buffer/recovery files. The interrupt test signals an
empty editor and checks only alternate-screen restoration. These checks can pass
without exercising the claimed behavior and can race on slower systems.
`docs/roles.md` identifies arbitrary sleeps and weakened or incomplete
assertions as material review findings.

**Required resolution:** Synchronize PTY actions on observable terminal state or
protocol responses instead of fixed sleeps. Assert scroll effects for both
focused panes, and check that interrupting after edits leaves the configured
directory free of buffer, backup, and recovery state. Keep the terminal
restoration assertions.

Evidence: [`pty_test.go`](/home/nevd/mdiff/internal/prototype/pty_test.go:36),
[`pty_test.go`](/home/nevd/mdiff/internal/prototype/pty_test.go:53), and
[`pty_test.go`](/home/nevd/mdiff/internal/prototype/pty_test.go:87).

Resolution: PTY checks now wait for rendered screen and cursor checkpoints,
assert scrolling from both focused panes, and interrupt after pasting content
before checking terminal restoration and an empty config directory.

### 3. The current format-check fails on the implementation plan

**Severity:** Medium\
**Status:** Resolved in pass 2

The latest `just lint` run stopped in `just format-check` with
`design_docs/implementation_plan.md` reported as unformatted. This prevents the
required lint gate from passing on the current workspace, despite the handoff
reporting all Linux gates as passed.

**Required resolution:** Run the repository formatter on the plan and rerun
`just format-check` and `just lint` on the resulting workspace.

Evidence: `just lint` / `just format-check` output from this review pass.

Resolution: the implementation plan is now formatted and the complete
`just lint` recipe passes.

## Review

The selected Micro source closure, transient buffer setup, line alignment, and
shared viewport implementation match TASK-002's architecture and scope. The pass
1 cursor, PTY coverage, and format findings are resolved. Simulation and PTY
coverage and the required Linux gates now satisfy the task's acceptance
criteria. TASK-002 is approved.

## Verification, review pass 1

- Initial `just test`, `just test-pty`, and `just build` could not use Go's
  default cache because it is under a read-only home directory. Re-ran with
  `GOCACHE=/tmp/mdiff-review-gocache`:
  - `just test`: passed.
  - `just test-pty`: passed.
  - `just build`: passed.
- `GOCACHE=/tmp/mdiff-review-gocache GOLANGCI_LINT_CACHE=/tmp/mdiff-review-golangci-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`:
  passed as part of the first `just lint` attempt.
- `/tmp/mdiff-gobin/golangci-lint run --timeout=5m ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`:
  passed, 0 issues. `golangci-lint` was not on `PATH` for `just lint`.
- `just lint-markdown`: passed.
- `just lint-yaml`: passed.
- Docker Markdownlint on `review_docs/TASK-002.md`: passed.
- Pinned `mdformat --check review_docs/TASK-002.md`: passed.
- The latest complete `just lint` attempt failed in format-check because
  `design_docs/implementation_plan.md` is not formatted. An earlier attempt
  passed format-check and vet but stopped when `golangci-lint` was unavailable
  on `PATH`.
- macOS CI verification was not available in this review environment.

## Verification, review pass 2

- `GOCACHE=/tmp/mdiff-review2-gocache just test`: passed.
- `GOCACHE=/tmp/mdiff-review2-gocache just test-pty`: passed.
- `GOCACHE=/tmp/mdiff-review2-gocache just build`: passed.
- `PATH=/tmp/mdiff-gobin:$PATH GOCACHE=/tmp/mdiff-review2-gocache GOLANGCI_LINT_CACHE=/tmp/mdiff-review2-golangci-cache just lint`:
  passed, including format-check, vet, golangci-lint (0 issues), Markdownlint,
  and yamllint.
- Docker Markdownlint and pinned `mdformat --check` on this updated review
  record: passed.
- The first concurrent `just lint` attempt collided with the separate
  golangci-lint process; the subsequent standalone full recipe passed.
- macOS CI verification was not available in this review environment.
