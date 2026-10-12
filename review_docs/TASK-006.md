# TASK-006 Review

**Task:** Load Micro configuration safely  
**Reviewer:** Rufus  
**Review pass:** 1  
**Decision:** CHANGES REQUIRED

## Findings

### 1. Medium — `Unsplit` binding can panic in mdiff

**Status:** Open

`Unsplit` is present in Micro's buffer action map, but it is missing from
`prototypeUnsafeAction`. A user can bind an otherwise ordinary key to
`Unsplit`; mdiff accepts the binding and invokes the action. mdiff constructs
each `BufPane` with a nil Micro tab, so `BufPane.Unsplit` dereferences that nil
tab and panics. The task requires rejecting actions that are unavailable in
mdiff's exactly-two-pane model and reporting an actionable diagnostic.

**Required resolution:** Reject `Unsplit` as an unavailable binding, with the
usual status diagnostic, and add a regression test that loads such a binding
and verifies invoking it neither panics nor changes the two-pane layout.

Evidence: [`BufMapEvent`](/home/nevd/mdiff/internal/micro/action/bufpane.go:119)
filters actions through `prototypeUnsafeAction`;
[`prototypeUnsafeAction`](/home/nevd/mdiff/internal/micro/action/bufpane.go:231)
does not include `Unsplit`;
[`NewBufPane`](/home/nevd/mdiff/internal/prototype/app.go:451) receives a nil
tab; and [`BufPane.Unsplit`](/home/nevd/mdiff/internal/micro/action/actions.go:2099)
dereferences `h.tab` immediately.

## Review

The configuration lookup and read-only startup, settings application to both
panes, keybinding loading and labels, colorscheme handling, synthetic
filetypes, and persistence settings have broad fixture coverage. This
unfiltered tab action remains a crash path for a valid user configuration and
blocks approval.

## Verification

- `just format`: passed.
- `PATH=/home/nevd/go/bin:$PATH just lint`: passed, including Go lint (0
  issues), Markdownlint, and yamllint.
- `PATH=/home/nevd/go/bin:$PATH just test`: passed.
- `PATH=/home/nevd/go/bin:$PATH just test-pty`: passed.
- `PATH=/home/nevd/go/bin:$PATH just build`: passed.

Docker/runtime access for lint was obtained through escalation.
