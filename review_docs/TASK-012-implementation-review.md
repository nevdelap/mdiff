# TASK-012 implementation review — 2026-10-16

**Decision:** Ready for completion; no open findings after pass 3.\
**Scope:** The TASK-012 working change against its implementation plan and the
pinned Micro and tcell code. This review does not change implementation files.

## Pass 1 findings (resolved in pass 2)

### 1. High — A real screen restart error can panic before it is returned

[`screen.Init`](../internal/micro/screen/screen.go) calls `Screen.Fini()` when
`Screen.Init()` returns an error. The pinned tcell v2.0.13 terminal screen is
not safe to finalize before initialization completes: its `finish` method writes
through the terminal output and waits for an input-loop channel that may not yet
exist. I reproduced this with a valid `TERM` and no accessible TTY: tcell
`Init()` returned `open /dev/tty: no such device or address`, then `Fini()`
panicked with a nil writer. TASK-012's intended error-returning restart path is
therefore bypassed under a genuine initialization failure. The current test
replaces the entire restart function with an error stub, so it cannot catch this
failure.

**Required change:** Make the Micro screen initializer clear its failed screen
without calling tcell `Fini()` on a screen whose `Init()` did not succeed. Keep
the terminal-session error path intact. Add a failure test that exercises the
real `screen.Init` branch through a controlled tcell initialization failure and
checks that no panic, stale screen use, or terminal-mode leak occurs.

### 2. Medium — The new shell executor duplicates and changes Micro's command behavior

[`RunInteractiveCommand`](../internal/micro/shell/shell.go) repeats the command
parsing, process setup, output capture, and Enter prompt already in
`RunInteractiveShell`. The two paths already differ: Micro's original path waits
for Enter after a command-start failure even with `wait=false`; the new path
does not. The new path also prints a nonzero exit error directly to the terminal
when `wait=false`, whereas the original returns it without printing. The plugin
bridge exposes `RunInteractiveShell` with these parameters, so the difference
can reach users outside the default `ShellMode` binding. This conflicts with
TASK-012's requirement to keep Micro's command semantics and mdiff's goal of
minimal adaptation.

**Required change:** Put the shared command execution and output/wait behavior
in one Micro-owned path. Let mdiff provide only terminal and signal lifecycle
hooks and the required visible failure reporting. Cover command-start failure,
nonzero exit, `wait=false`, and `getOutput=true` through the app route.

### 3. Medium — The PTY tests do not establish all required handoff behavior

The shell PTY test searches for `FIRST-SHELL-OUTPUT` and `SECOND-SHELL-OUTPUT`,
but each marker is also present in the echoed command line. The marker assertion
can pass without the command printing its output. The suspend PTY test edits the
left pane after resume but does not switch panes or check diff alignment. The
in-process handoff test checks one pending diff result, but does not assert
focus, cursor/selection, dirty state, or stale-result discard. The
restart-failure test does not inspect terminal modes. These are explicit
TASK-012 acceptance checks, and the current tests leave regressions in the
two-pane session undetected.

**Required change:** Distinguish command output from echoed input in the PTY
transcript and assert it stays visible until Enter. Extend the suspend/resume
and in-process cases to verify pane switching, aligned rows, buffer identity,
focus, selections, dirty state, and a stale diff result. Include terminal-mode
restoration in the real restart-failure test from finding 1.

## Verification

`GOCACHE=/tmp/mdiff-go-review-cache go test ./...` passed. The focused TASK-012
PTY and handoff tests also passed. A separate read-only tcell reproducer
confirmed the initialization-error panic described in finding 1. No
implementation code was changed.

## Pass 2 re-review

Igor's changes resolve all three pass 1 findings:

| Finding                            | Status   | Evidence                                                                                                                                                                                                                               |
| ---------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1. Restart failure                 | Resolved | `screen.Init` clears a failed tcell screen without calling `Fini`; new tests exercise a real `/dev/tty` initialization error, check error propagation and stale-screen state, and compare terminal modes before and after.             |
| 2. Shell duplication and semantics | Resolved | Both standalone Micro and mdiff use the same `executeInteractive` command body. App-route tests cover command-start failure, nonzero exit, `wait=false`, `getOutput=true`, and the terminal wait prompt.                               |
| 3. Handoff coverage                | Resolved | PTY output markers cannot be satisfied by command echo. The suspend test checks both panes, alignment, pane switching, edits, and quit. In-process tests cover buffer identity, focus, selection, dirty state, and stale diff results. |

### 4. Medium — Invalid shell input still causes an unnecessary terminal handoff

[`runInteractiveShell`](../internal/prototype/app_compat.go) calls `a.handoff`
before [`executeInteractive`](../internal/micro/shell/shell.go) parses the
command. Micro's original shell path parses first and returns on an empty or
malformed command without releasing its screen. In mdiff, submitting an empty
shell prompt or an unmatched quote releases and restarts the terminal even
though no process can start. A restart failure in this case exits the editor for
an input error that Micro handles without a handoff. The extra terminal
transition also changes the visible behavior of the default shell prompt.

**Required change:** Arrange the shared Micro-owned executor so it validates
input before calling mdiff's terminal handoff, without adding a second shell
parser in mdiff. Test an empty command and malformed quoting through the app
route; neither should release or restart the screen, and editing should remain
usable.

The focused screen, handoff, shell-route, and PTY tests passed. The full
`GOCACHE=/tmp/mdiff-go-review-cache go test ./...` suite also passed. No
implementation code was changed in this pass.

## Pass 3 re-review

Finding 4 is **resolved**. Micro's new `PrepareInteractiveCommand` parses and
validates the shell input once. mdiff calls it before `a.handoff`, and the
prepared command executes through the same Micro-owned path after terminal
release. The app-route test checks that empty input and malformed quoting cause
no handoff, leave the current screen in place, report the error, and allow the
next edit.

All four findings in this review are resolved. The TASK-012 implementation is
ready for completion under the stated acceptance criteria. The focused screen,
shell-route, handoff, and PTY tests passed, as did
`GOCACHE=/tmp/mdiff-go-review-cache go test ./...`. No implementation code or
task state was changed in this review pass.
