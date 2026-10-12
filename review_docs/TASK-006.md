# TASK-006 Review

**Task:** Load Micro configuration safely  
**Reviewer:** Rufus  
**Review pass:** 5  
**Decision:** APPROVED

## Findings resolved

### `Unsplit` binding could panic

**Status:** Resolved in pass 2

`Unsplit` is rejected by `prototypeUnsafeAction`. A regression test loads a
user binding to `Unsplit`, invokes it, and checks that mdiff stays open, retains
both buffers and its layout, and reports that the action is unavailable.

Evidence: [`prototypeUnsafeAction`](/home/nevd/mdiff/internal/micro/action/bufpane.go:231)
and [`TestConfiguredUnsplitActionIsRejectedWithoutChangingPanes`](/home/nevd/mdiff/internal/prototype/editing_test.go:418).

### Micro private-view scroll bindings

**Status:** Resolved in pass 5

`ScrollUp`, `ScrollDown`, `Center`, `Start`, `End`, `PageUp`, `PageDown`,
`HalfPageUp`, and `HalfPageDown` are rejected because they change a private
Micro pane viewport that mdiff does not render. The configured-binding test
checks that each action reports unavailable and leaves the shared viewport
unchanged. The compatibility matrix lists the exclusions.

Evidence: [`prototypeUnsafeAction`](/home/nevd/mdiff/internal/micro/action/bufpane.go:231),
[`TestConfiguredPrivateViewportActionsAreRejected`](/home/nevd/mdiff/internal/prototype/editing_test.go:448),
and [`bindings.json` compatibility row](/home/nevd/mdiff/design_docs/micro_diff_editor.md:550).

## Review

Configuration lookup and read-only startup, settings application, colorscheme
and syntax loading, lifecycle action filtering, and shared-viewport binding
behavior meet TASK-006's acceptance criteria. No open findings remain.

## Verification

- `just format`: passed.
- `PATH=/home/nevd/go/bin:$PATH just lint`: passed, including Go lint (0
  issues), Markdownlint, and yamllint.
- `PATH=/home/nevd/go/bin:$PATH just test`: passed.
- `PATH=/home/nevd/go/bin:$PATH just test-pty`: passed.
- `PATH=/home/nevd/go/bin:$PATH just build`: passed.

Docker/runtime access for lint was obtained through escalation.
