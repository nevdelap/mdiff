# TASK-011 Review

**Task:** Implement Micro-compatible per-pane file workflows  
**Reviewer:** Rufus  
**Review pass:** 4  
**Decision:** APPROVED

## Findings

### 1. High — Save flows could overwrite externally changed files

**Status:** Resolved in pass 4

The prior pass found that quit and dirty Open/New Tab used `SaveCB` directly,
bypassing the external-change check. These paths now call
`SaveWithExternalChangeCheck`, as does Ctrl+S. Quit/autosave, dirty replacement,
and explicit saves therefore use the same per-buffer reload behavior. The
callback proceeds after a confirmed reload or a save; Escape cancels the
pending save/callback.

Simulation tests cover stale same-path buffers during explicit save, dirty
quit, and autosave quit, and external changes during dirty Open and New Tab.
They verify the on-disk contents remain unchanged while the decision is
pending, and check the results of reload, overwrite, and cancel choices.

Evidence: [`SaveWithExternalChangeCheck`](/home/nevd/mdiff/internal/micro/action/actions.go:987)
now accepts action and callback parameters;
[`saveForQuit`](/home/nevd/mdiff/internal/prototype/app.go:705) and
[`newFocusedDocument`](/home/nevd/mdiff/internal/prototype/app.go:734) use it.
Coverage is in [`file_workflow_test.go`](/home/nevd/mdiff/internal/prototype/file_workflow_test.go:48)
and [`quit_test.go`](/home/nevd/mdiff/internal/prototype/quit_test.go:195).

## Review

The per-pane file workflows, same-path conflict handling, modified-buffer quit
decisions, failure preservation, and two-pane adaptations meet TASK-011's
acceptance criteria. No open findings remain.

## Verification

- `just format`: passed.
- `PATH=/home/nevd/go/bin:$PATH just lint`: passed, including Go lint (0
  issues), Markdownlint, and yamllint.
- `PATH=/home/nevd/go/bin:$PATH just test`: passed.
- `PATH=/home/nevd/go/bin:$PATH just test-pty`: passed.
- `PATH=/home/nevd/go/bin:$PATH just build`: passed.
- `PATH=/home/nevd/go/bin:$PATH just vet`: passed.

Docker/runtime access for lint was obtained through escalation.
