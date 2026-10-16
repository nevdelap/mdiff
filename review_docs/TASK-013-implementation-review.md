# TASK-013 implementation review — 2026-10-16

**Decision:** Ready for completion; no open findings after pass 2.\
**Scope:** The TASK-013 working change, its command audit, and the relevant
pinned Micro handlers. No implementation files were changed in this review.

## Finding

### 1. Medium — `tabswitch` repeats tab activation after mdiff resizes

The audit allows `tabswitch` because mdiff has only one tab. Micro's
[`TabSwitchCmd`](../internal/micro/action/command.go) calls `Tabs.SetActive(0)`
for `tabswitch 1`. That method calls the Lua `onSetActive` hook whenever the
target tab's `isActive` flag is false. Micro's `TabList.Resize` keeps the
current `Tab` instance and its active flag. mdiff's
[`App.resize`](../internal/prototype/app.go) instead constructs a new
`action.Tab` and calls `microTab.SetActive` for the focused **pane**, but never
calls `Tabs.SetActive` for the **tab**. After each mdiff resize, the new tab's
`isActive` flag is false. The next `tabswitch 1` therefore fires `onSetActive`
again even if that hook already ran before the resize. A plugin can observe or
act on this extra activation. Micro's normal resize followed by selecting its
already-active tab would skip the hook.

The new test sets `app.focus` directly without synchronizing the Micro tab's
active pane and does not exercise a resize or register an `onSetActive` hook, so
it misses this behavior difference. The command offers no navigation in a
one-tab editor.

**Required change:** Keep `tabswitch` guarded in mdiff and document that there
is no second tab to select; remove its allowed-command claim and test. This is
the smaller change and avoids adding tab activation state handling solely for a
no-op command. Keep `eval` routed to Micro's own unsupported message.

## Other checks

The nine remaining guarded commands have source-backed reasons in the technical
design. The `reload` explanation identifies both the uncoordinated Micro
settings/view lifecycle and mdiff's startup-only diff debounce. The command
guard, README, release note, and single patch-version bump agree. The full
`GOCACHE=/tmp/mdiff-go-review-cache go test ./...` suite passed.

## Implementation response to pass 1

- Restored the `tabswitch` guard in `safeCommand`; `eval` still reaches Micro.
- The routing regression now resizes mdiff's tab, then verifies that
  `tabswitch 1` is visibly rejected without changing the paired panes or shared
  viewport. It also synchronizes the focused Micro pane before routing.
- Updated the README, technical design, release note, future-work summary, and
  task handoff to explain the redundant `onSetActive` callback. The existing
  patch-version bump is unchanged.
- Verification passed after the fix: `just format`, `just lint`, `just test`,
  `just build`, and `just test-pty` on Linux.

## Pass 2 re-review

Finding 1 is **resolved**. `safeCommand` again guards `tabswitch`; the routing
test covers its visible rejection after a resize while preserving the two
buffers, panes, focus, and shared viewport. The technical design, README,
future-work summary, handoff, and release note now agree that `tabswitch` is
blocked and `eval` reaches Micro. The `reload` decision remains source-backed,
and the patch version is bumped once.

TASK-013 is ready for completion. The full
`GOCACHE=/tmp/mdiff-go-review-cache go test ./...` suite passed in this pass. No
implementation code or task state was changed by this review.
