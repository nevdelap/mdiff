# Micro behavior parity review — 2026-10-16

**Current decision (pass 8): approved.** Findings 1–9 and A–J are resolved and
covered by regressions. This review compares mdiff with the **bundled** Micro
v2.0.15 source, not an unpinned later release. It is a behavioral review; no
implementation or test files were changed by the reviewer.

The justified adaptations are two fixed, independently editable documents; one
vertically aligned diff viewport; diff navigation, highlighting, and
configuration; and the terminal, prompt, and layout handling needed to keep
those two panes intact. The findings below concern Micro behavior that those
requirements do not require changing. Severity reflects user impact, not the
size of the proposed fix. None is an optional enhancement.

## Findings

### 1. High — `QuitAll` no longer means Micro's discard-all operation

Micro [`QuitAll`](../internal/micro/action/actions.go) asks **once** whether to
close all modified buffers **without saving**. mdiff routes `Quit` and `QuitAll`
to the same `requestQuit` flow
([`app_compat.go`](../internal/prototype/app_compat.go)), which asks separately
whether to save each modified pane and may write both files
([`app_files.go`](../internal/prototype/app_files.go)). A `QuitAll` binding can
therefore save data when its Micro counterpart explicitly discards it, and has a
different confirmation sequence. The fixed two-pane design justifies adapting
`Quit`, but does not require changing the meaning of `QuitAll`.

**Required:** Give `QuitAll` its own one-confirmation, discard-both path while
preserving the existing save/discard/cancel behavior of ordinary `Quit`.
Exercise dirty left, dirty right, both dirty, and cancellation through a bound
`QuitAll` action.

### 2. High — Default diff keys lose their Micro fallback and navigation can repeat a hunk

Both bundled Micro default binding sets map `Alt-[` to
`DiffPrevious|CursorStart` and `Alt-]` to `DiffNext|CursorEnd`
([`defaults_other.go`](../internal/micro/action/defaults_other.go),
[`defaults_darwin.go`](../internal/micro/action/defaults_darwin.go)). Micro's
`DiffPrevious`/`DiffNext` return false when there is no target, so the binding
falls through to cursor start/end
([`actions.go`](../internal/micro/action/actions.go),
[`bufpane.go`](../internal/micro/action/bufpane.go)). mdiff's adapter returns
true even when `jumpDiff` found no hunk
([`app_compat.go`](../internal/prototype/app_compat.go),
[`app.go`](../internal/prototype/app.go)). With equal files, both shortcuts now
do nothing. Returning false from the adapter alone would call Micro's own VCS
diff action before the fallback, so the adapter needs a distinct *handled but
unsuccessful* result or equivalent binding dispatch.

`jumpDiff` also chooses the next hunk using `a.top`, whereas Micro searches from
the focused cursor line. When the final hunk starts below the maximum viewport
top, `clampTop` leaves `a.top` before that hunk. A second `DiffNext` then
selects the **same** hunk again. Scrolling without moving the cursor can
likewise change which hunk the next key chooses. Using the paired hunk list is
justified; losing cursor-relative navigation is not.

**Required:** Search paired hunks from the focused cursor's aligned position,
preserve Micro's fallback when no target exists, and prevent repeat selection at
the end. Test equal files, a final hunk near the bottom, scrolling before
navigation, and both focus sides.

### 3. High — Default page keys use a private viewport that mdiff never displays

Micro's default `PageUp`/`PageDown` keys invoke `CursorPageUp`/
`CursorPageDown`; `ShiftPageUp`/`ShiftPageDown` invoke `SelectPageUp`/
`SelectPageDown` (both default binding files). Those actions move Micro's cursor
or selection **and** its pane's `StartLine`
([`actions.go`](../internal/micro/action/actions.go)). mdiff adapts the
scroll-only `PageUp`/`PageDown`, but leaves the four default cursor/selection
actions to Micro ([`app_compat.go`](../internal/prototype/app_compat.go)). Its
renderer instead displays rows from `a.top`, the shared viewport
([`render.go`](../internal/prototype/render.go)). After a default page key,
`ensureCursorVisible` merely brings the cursor into view; the Micro action's
intended relative cursor position is lost. This is most visible when the cursor
starts in the middle of a long file: Micro pages both cursor and view by the
same amount, while mdiff can place the cursor at the first or last visible row.

**Required:** Adapt these four actions to the shared aligned viewport while
retaining Micro's cursor movement, selection, `pageoverlap`, and visual-row
rules. Test both directions, selection, wrapping, and rows inserted on only one
side.

### 4. Medium — Command names and quoting no longer follow Micro's parser

Micro parses commands with `shellquote.Split` and looks up the resulting name
exactly; plugin commands registered by `MakeCommand` therefore retain their case
([`command.go`](../internal/micro/action/command.go)). mdiff first uses
`strings.Fields`, lowercases the first field, then checks registration
([`app_compat.go`](../internal/prototype/app_compat.go),
[`app.go`](../internal/prototype/app.go)). A plugin command registered as
`MyCommand` runs in Micro but is rejected in mdiff. A quoted command name such
as `"help"` reaches Micro's parser but is rejected before mdiff can dispatch it.
The same pre-parser can report `unavailable` for malformed input where Micro
reports a parse error. This affects both the command bar and bound `command:`
actions; it is unrelated to paired diff behavior.

**Required:** Parse once with Micro's shell quoting rules, preserve exact
registered names, then guard only the exact commands that would break the fixed
layout. Cover mixed-case plugin names, quoted names/arguments, malformed quotes,
and the existing blocked-command list.

### 5. Medium — Help is a separate editor with hard-coded controls

Micro opens help as a `BTHelp` buffer and runs normal buffer bindings and
rendering ([`command.go`](../internal/micro/action/command.go),
[`actions.go`](../internal/micro/action/actions.go)). mdiff opens raw help text
in a custom overlay, wraps and paints it independently, handles `Ctrl+F`,
`n`/`N`, and navigation keys directly, and closes on other input
([`app_help.go`](../internal/prototype/app_help.go),
[`app_render.go`](../internal/prototype/app_render.go),
[`app.go`](../internal/prototype/app.go)). The search *pattern compilation* now
uses Micro's helper, but remapped `Find`/`FindNext`, other configured help
bindings, Micro help syntax, and buffer selection/copy cannot work the way they
do in Micro. The overlay can preserve the fixed two-document layout without
replacing Micro's help input and text behavior. The intercepted `help` command
also accepts at most one topic, unlike Micro's multi-topic and split-option
parser; extra help splits must be adapted to a single overlay, but rejecting
otherwise valid topics is not required by diff functionality.

**Required:** Reuse Micro's help buffer/rendering and applicable input actions
inside the paired-layout overlay, adapting only the split presentation. Cover
remapped search/navigation, help syntax, and multiple requested topics.

### 6. Medium — `SaveAll` has different save semantics

Micro's [`SaveAll`](../internal/micro/action/actions.go) calls `Save()` on every
open buffer. mdiff's [`saveAll`](../internal/prototype/app_compat.go) skips
unmodified buffers and uses `SaveWithExternalChangeCheck`, which can prompt and
defer the remaining saves. The two-pane app justifies iterating its two
documents, but not a different meaning for the action. The current test only
checks two modified named files and cannot distinguish the implementations
([`behavior_compat_test.go`](../internal/prototype/behavior_compat_test.go)).

**Required:** Make `SaveAll` use Micro's save semantics for both mdiff buffers,
including untitled and externally changed cases, and test its behavior against
the bundled Micro action. If Micro's own behavior is intentionally changed, make
that a separate explicit product decision rather than an unnoticed parity claim.

### 7. Medium — Mouse wheel bindings run in the focused pane, not the pane under the pointer

Micro's tab dispatcher sends wheel events to the pane at the mouse position
without changing which pane is active
([`tab.go`](../internal/micro/action/tab.go)). mdiff sends every wheel event to
`focusedPane()` regardless of pointer position
([`app.go`](../internal/prototype/app.go)). With the default vertical scroll
bindings, the shared viewport masks the difference. A user binding such as
`MouseWheelDown: CursorEnd` moves the wrong document's cursor when the wheel is
over the inactive pane. The shared vertical scroll does not require that change
to event ownership.

**Required:** Dispatch wheel events to the pane under the pointer, preserving
the shared-scroll adapter for scroll actions. Test a non-scroll wheel binding
over the inactive pane.

### 8. Medium — The configured Micro scrollbar is silently disabled

mdiff forces `scrollbar=false` on both initial buffers and every replacement
buffer ([`app.go`](../internal/prototype/app.go),
[`app_files.go`](../internal/prototype/app_files.go)). Micro's renderer uses the
setting to reserve a column and draw the configured scrollbar
([`bufwindow.go`](../internal/micro/display/bufwindow.go)). mdiff paints each
source row through a temporary two-row Micro window, so this feature needs a
shared-viewport-aware composition step; forcing the user's setting off does not
provide Micro parity. The separate `diffgutter=false` override also merits an
explicit product decision because it suppresses Micro's VCS diff display,
although that one may be justified by mdiff's own diff gutter.

**Required:** Honor the Micro `scrollbar` setting in each pane using the shared
viewport's position and height; preserve its configured character and style.
Test startup settings and file replacement. Decide explicitly whether
`diffgutter` is replaced by mdiff's diff presentation and document that
exception if so.

### 9. Medium — Plugin `config.Reload` bypasses mdiff's reload guard

mdiff blocks the `reload` command because Micro's reload does not update the
active diff coordinator and paired viewport safely
([`app.go`](../internal/prototype/app.go), [`README.md`](../README.md)). The Lua
bridge still exposes Micro's `config.Reload` as `action.ReloadConfig`
([`plugins.go`](../internal/prototype/plugins.go)), which executes
`reloadRuntime(false)`, rereads settings, reinstalls bindings and commands, and
reloads open-buffer settings
([`command.go`](../internal/micro/action/command.go)). A plugin can therefore
change layout-affecting settings through the path the command guard prohibits,
without rebuilding mdiff's coordinator or viewport. This is a compatibility and
layout-safety gap, not a reason to remove Micro plugin support.

**Required:** Route plugin reload through the same mdiff-aware policy as the
command, or implement a safe shared reload path. Test a plugin reload after a
wrapping or diff-debounce setting change.

## Coverage and verification

The current tests cover substantial Micro integration: ordinary editing, search,
selection, mouse gestures, save/open/quit prompts, configuration, plugins,
terminal handoff, and diff rendering. They primarily assert mdiff's present
behavior. In particular, there are no parity regressions for the action
success/fallback chain, `QuitAll`, default page keys, wheel routing, command
quoting and case, help remaps, or scrollbar restoration. Passing them cannot
establish the requested *same as Micro except diff* contract.

For each finding, add a focused comparison against the bundled Micro action or a
table of Micro-observed input/output behavior, then assert the mdiff result with
only the two-pane/diff adaptation changed. Keep the cases above as permanent
regressions. A full action-by-action equivalence claim would also need
representative configured bindings, prompts, both focus sides, and terminal
resizes; source inspection alone cannot prove every plugin or terminal behavior
identical.

Verification performed for this review:

- Source comparison of mdiff's event, command, action, file, rendering, help,
  and plugin adapters with the bundled Micro implementations and default
  bindings.
- `GOCACHE=/tmp/mdiff-go-parity-cache go test ./...` — passed.

**Review boundary:** Existing task-014 implementation findings remain in
[`TASK-014-implementation-review.md`](TASK-014-implementation-review.md). This
document records only current cross-cutting Micro behavior parity findings.

## Pass 2 — review of TASK-015 fixes

The implementation now separates `QuitAll`, preserves action-chain success for
diff navigation, parses commands with Micro's shell quoting, uses a Micro help
buffer and window, calls `Buffer.Save` for both documents, routes wheel events
by horizontal pane position, composes scrollbars, and guards plugin reload. The
new tests cover several of these paths. The following statuses supersede the
original findings above; the original text remains as the pass-1 record.

| Finding           | Status   | Review result                                                                                                                                                                 |
| ----------------- | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1. `QuitAll`      | Resolved | `requestQuitAll` gives one discard confirmation, and the bound-action tests cover dirty left, right, both, cancellation, and acceptance.                                      |
| 2. Diff keys      | **Open** | No-hunk fallback and cursor-relative search are fixed. Navigation still repeats a trailing hunk that exists only in the opposite pane; see A below.                           |
| 3. Page keys      | **Open** | The adapter copies Micro's private `StartLine` after a page action, but that line can be stale relative to mdiff's shared viewport; see B below.                              |
| 4. Command parser | Resolved | `shellquote.Split` and exact command names now reach Micro's parsed-command dispatcher; tests cover mixed-case plugin commands, quoted arguments, and malformed quoting.      |
| 5. Help           | **Open** | Micro now renders and searches the help buffer, but default help/quit actions can enter unsafe tab operations; paste still edits the hidden document; see C below.            |
| 6. `SaveAll`      | Resolved | The adapter calls `Buffer.Save` for both documents. Tests distinguish untitled and externally changed cases from the old prompt-based path.                                   |
| 7. Mouse wheel    | **Open** | Routing works inside a pane, but wheel events over the divider/footer go to a pane selected by x rather than Micro's active-pane fallback; see D below.                       |
| 8. Scrollbar      | Resolved | The configured scrollbar is retained on initial and replacement buffers and rendered using Micro's character/style. The VCS `diffgutter` replacement is stated in the README. |
| 9. Plugin reload  | Resolved | Lua `config.Reload` now reaches mdiff's guarded path, with a regression for the blocked call.                                                                                 |

### A. High — Diff navigation repeats an opposite-side-only hunk at end of file

[`jumpDiff`](../internal/prototype/app.go) now searches from the focused cursor,
but if a target hunk contains only spacers on that side, it moves the cursor to
a neighboring real line. For a right-only insertion after the left file's last
line, a left-focused `DiffNext` selects the trailing hunk, moves the cursor back
to that last left line, and returns success. The next `DiffNext` sees the same
cursor row before the same hunk and selects it again. The current fallback test
([`behavior_compat_test.go`](../internal/prototype/behavior_compat_test.go))
uses equal files and checks `DiffPrevious` only; it does not exercise this
one-sided end case.

**Required:** Track the navigated aligned hunk when the focused side has no line
in it, so the next/previous action can advance or report failure and run its
Micro fallback. Add trailing and leading one-sided hunk cases from both focus
sides, including repeated key presses.

### B. High — Page actions can jump to the wrong shared viewport after scrolling

The new page adapter invokes Micro's cursor/selection page action and copies
`p.view.StartLine` into `a.top`
([`app_compat.go`](../internal/prototype/app_compat.go)). mdiff's wheel and
shared `ScrollDown`/`ScrollUp` actions change `a.top` without changing
`p.view.StartLine`. After scrolling down in a long equal file, a default
`PageDown` starts Micro's page calculation from that stale private position,
then installs it as the shared top. `ensureCursorVisible` may pull the view back
to the cursor, but it cannot preserve Micro's relative cursor position. The new
tests call `GotoLoc` before paging, which updates Micro's private view and
avoids the stale-view scenario.

**Required:** Synchronize the focused Micro view to the current aligned top
before a page action, then translate the resulting view back to aligned rows,
including one-sided spacers and wrap rows. Add a wheel/scroll-then-page
regression that checks the cursor's relative screen row, not only that the
cursor and `a.top` increased.

### C. High — Help actions can invoke Micro tab teardown on a detached pane

[`openHelpTopics`](../internal/prototype/app_help.go) creates `helpPane` with
`a.microTab`, but the help pane is not in that tab's two-pane list. The new
[`routeMicroAction`](../internal/prototype/app_compat.go) deliberately handles
only the left and right document panes. While help is visible, the default
`Ctrl+G` (`ToggleHelp`) or `Ctrl+Q` (`Quit`) therefore runs Micro's original
action on the detached help pane. `ToggleHelp` calls `Quit` for `BTHelp`; `Quit`
calls `ForceQuit`; `ForceQuit` calls `Unsplit` because the associated tab has
two panes ([`actions.go`](../internal/micro/action/actions.go)). That operation
expects a real tab node for the help pane and can tear down the fixed layout or
panic instead of closing the overlay.

There is a second dispatch gap: [`App.handle`](../internal/prototype/app.go)
routes help key and mouse events to `helpPane`, but an `EventPaste` with no
prompt still goes to the focused document pane. A paste made while help covers
the editor therefore edits a hidden document. A raw bound event closes help
without running its Micro help binding. The new help regression covers a
remapped `Find`/`FindNext`, but none of these cases.

**Required:** Give help-pane file, quit, tab, and toggle actions safe overlay
semantics without calling Micro's tab teardown; route paste and raw events to
the visible help context. Regress default `Ctrl+G`, `Ctrl+Q`, paste, and a raw
binding while help is open, asserting that both documents and the fixed tab
survive. Also retain Micro's error for conflicting `-vsplit` and `-hsplit`
flags; the new parser currently silently accepts both.

### D. Medium — Wheel events outside either pane target the wrong pane

Micro's [`Tab.HandleEvent`](../internal/micro/action/tab.go) scans pane bounds
for a wheel event and falls through to the active pane when no bounds match.
mdiff's [`handleMouse`](../internal/prototype/app.go) chooses left or right
solely from x, ignoring y and the center divider. With focus on the left,
`MouseWheelDown: CursorEnd` over the right half of the footer moves the right
cursor; Micro would move the active left cursor. Inside-pane wheel routing is
now correct, but the new test exercises only that case.

**Required:** Use actual pane bounds and fall back to the focused pane on the
divider and outside the editor rows. Test a non-scroll wheel binding in those
locations.

Verification for pass 2:

- `GOCACHE=/tmp/mdiff-go-review-fixes-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-review-fixes-cache go build -buildvcs=false -o /tmp/mdiff-review-build ./cmd/mdiff`
  — passed.
- `GOCACHE=/tmp/mdiff-go-review-fixes-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.

**Decision:** TASK-015 remains in review until A–D are resolved and covered.

## Implementation response to pass 2

The four open findings are addressed in the same TASK-015 change, with no
additional patch version bump:

- **A — addressed:** Diff navigation records the selected aligned hunk together
  with the focused side, cursor, content versions, and layout. Repeated next or
  previous navigation advances from that hunk even when it has no source line in
  the focused pane. `TestDiffNavigationAdvancesPastOneSidedBoundaryHunks` covers
  leading and trailing hunks from both pane sides.
- **B — addressed:** Before each Micro page action, the focused Micro window is
  positioned at the source cell corresponding to mdiff's shared top. The action
  then runs with Micro's own page height, overlap, movement, and selection
  behavior; its resulting start line maps back into aligned rows.
  `TestPageActionAfterSharedWheelScrollPreservesCursorScreenRow` leaves the
  private Micro view stale after shared scrolling and checks the cursor's
  displayed row against Micro's resulting view.
- **C — addressed:** Help actions for save, open, quit, tab, split, and toggle
  operations now close or operate through the overlay policy rather than
  invoking tab teardown on the detached pane. Paste and raw events dispatch to
  the visible help pane. Conflicting `-vsplit` and `-hsplit` flags report
  Micro's error. `TestHelpOverlayContainsQuitPasteAndRawActions` covers the
  close actions, paste, a raw binding, and conflicting flags; the existing help
  regression covers remapped `Find` and `FindNext`.
- **D — addressed:** Wheel dispatch checks each pane window's x/y bounds and
  uses the focused pane when the pointer is on the divider, footer, or outside
  the editor. `TestMouseWheelOutsidePaneBoundsFallsBackToFocusedPane` covers
  those three locations.

Verification after these changes:

- `GOCACHE=/tmp/mdiff-go-review-fixes-cache go test ./...` — passed.
- `just lint` — passed.
- `go build -buildvcs=false -o /tmp/mdiff-review-fixes-build ./cmd/mdiff` —
  passed.

## Pass 3 — independent review of the four fixes

The four pass-2 findings **A–D are resolved** in the current TASK-015 change.
The paired-hunk navigation state handles repeated keys at one-sided boundaries;
the page adapter synchronizes Micro's private start before paging; help quit,
toggle, raw, and paste events stay within the overlay; and wheel dispatch uses
actual pane bounds with an active-pane fallback. The new regressions exercise
the reported cases. This pass found two additional parity defects in the new and
surrounding help/navigation paths.

| Original finding  | Current status | Result                                                                                                                                     |
| ----------------- | -------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| 1. `QuitAll`      | Resolved       | Pass-2 fix retained.                                                                                                                       |
| 2. Diff keys      | **Open**       | Pass-2 boundary issue A is fixed, but new finding F below shows stale navigation state after a plugin replaces a buffer.                   |
| 3. Page keys      | Resolved       | Pass-2 issue B is fixed for shared scrolling; the page action now starts from the visible aligned top.                                     |
| 4. Command parser | Resolved       | Pass-2 fix retained.                                                                                                                       |
| 5. Help           | **Open**       | Pass-2 safety issue C is fixed, but new finding E below shows that help still changes Micro's text and ignores a configured `Esc` binding. |
| 6. `SaveAll`      | Resolved       | Pass-2 fix retained.                                                                                                                       |
| 7. Mouse wheel    | Resolved       | Pass-2 issue D is fixed for pane, divider, footer, and outside positions.                                                                  |
| 8. Scrollbar      | Resolved       | Pass-2 fix retained.                                                                                                                       |
| 9. Plugin reload  | Resolved       | Pass-2 fix retained.                                                                                                                       |

### E. Medium — Help adds source text and intercepts a configured binding

For a single topic, [`openHelpTopics`](../internal/prototype/app_help.go)
prepends `"# "+topic+"\n\n"` to the runtime page before creating its `BTHelp`
buffer. Micro's [`openHelp`](../internal/micro/action/command.go) creates the
buffer from the runtime page's bytes directly. The bundled `commands.md` already
starts with its own heading, so `help commands` in mdiff has an extra heading
and two extra lines. This shifts line numbers, cursor positions, and search
results in a view that is supposed to use Micro's help text. A temporary review
probe compared `helpBuf.Bytes()` with the runtime page and failed on the extra
text.

[`handleHelpKey`](../internal/prototype/app_help.go) also closes the overlay
whenever it receives `Esc`, before passing the event to Micro's binding tree.
Micro's default `Esc` is a chain of `Escape`, `Deselect`, `ClearInfo`, and
related actions; it does not close the help buffer. More concretely, remapping
`Esc` to `CursorEnd` moves the cursor in Micro but closes mdiff help without
running the binding. A temporary review probe confirmed this. `Ctrl+G` now
closes the mdiff help overlay safely, so this hard-coded interception is not
needed to leave help.

**Required:** Use the exact bundled page text for a single help topic, adding
topic separators only where the fixed-layout representation of multiple pages
needs them. Dispatch `Esc` through Micro's configured help bindings. Add
regressions for exact single-page bytes and a remapped `Esc` action.

### F. High — Diff navigation state survives replacement with a different buffer

[`jumpDiff`](../internal/prototype/app.go) retains a hunk index when focused
side, cursor location, content versions, and layout key match. Replacing a
buffer can preserve all four values: freshly opened files both start at content
version zero and can have the same viewport settings. Neither
[`syncPaneBuffer`](../internal/prototype/app_files.go) nor the new navigation
state records the buffer identity. A plugin timer can replace the focused Micro
buffer and call `syncChangedBuffers` without a bound action resetting
`diffNavValid`; the app's timer path does exactly that synchronization
([`app.go`](../internal/prototype/app.go)).

A temporary review probe opened files with an old hunk at line 0, navigated to
it, replaced the focused buffer with a new file containing hunks at lines 1 and
3, and synchronized the panes as the plugin-timer path does. The next `DiffNext`
jumped to **line 3**, skipping the first hunk at line 1. The regular command-bar
path resets navigation state on its `CommandMode` action, which is why existing
open-file tests do not expose this case.

**Required:** Invalidate hunk-navigation state whenever either pane changes
buffer identity or the installed hunk list is rebuilt, or include buffer
identity in the continuation guard. Add a plugin-style replacement regression
with equal content versions/layout and distinct old/new hunk lists.

Pass-3 verification:

- `GOCACHE=/tmp/mdiff-go-review-fixes-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-review-fixes-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-review-fixes-cache go build -buildvcs=false -o /tmp/mdiff-review-fixes-build ./cmd/mdiff`
  — passed.
- Two temporary `go test -overlay` review probes outside the repository — both
  failed as described in E and F. No implementation or test files were changed
  for these probes.
- `XDG_RUNTIME_DIR=/tmp/mdiff-review-runtime UV_OFFLINE=1 just lint` — could not
  complete: Docker socket access was denied during `format-check`. The
  implementer's handoff reports `just lint` passed in its environment.

**Decision:** TASK-015 remains in review until E and F are resolved and covered.

## Implementation response to pass 3

Both pass-3 findings are addressed in the existing TASK-015 change, with no
additional patch version bump:

- **E — addressed:** A single help topic now uses the bundled runtime page bytes
  unchanged. Multiple requested pages retain topic separators. `Esc` is
  dispatched through the help pane's configured Micro bindings rather than
  closing the overlay directly.
  `TestMicroHelpSingleTopicPreservesBytesAndEscBinding` checks exact single-page
  bytes and a remapped `Esc` action; existing tests cover multiple topics and
  the explicit help close action.
- **F — addressed:** Diff navigation continuation now includes both pane buffer
  identities. Replacing either buffer prevents a saved hunk index from carrying
  into the replacement's hunk list.
  `TestDiffNavigationResetsAfterPluginReplacesBuffer` simulates the plugin timer
  synchronization path with matching content versions/layout and verifies
  navigation selects the first replacement hunk.

Verification after these changes:

- `GOCACHE=/tmp/mdiff-go-pass3-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-pass3-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-pass3-cache go build -buildvcs=false -o /tmp/mdiff-pass3-build ./cmd/mdiff`
  — passed.

## Pass 4 — independent review of the final fixes

**Decision: approved.** Findings E and F are resolved, leaving no open finding
from this review. The statuses in this section supersede the earlier pass tables
and decisions; the earlier text remains as the review history.

| Finding                | Final status | Evidence                                                                                                                                                                                    |
| ---------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1. `QuitAll`           | Resolved     | Pass-2 fix retained.                                                                                                                                                                        |
| 2. Diff keys           | Resolved     | Pass-2 and pass-3 fixes retained; F is resolved.                                                                                                                                            |
| 3. Page keys           | Resolved     | Pass-3 fix retained.                                                                                                                                                                        |
| 4. Command parser      | Resolved     | Pass-2 fix retained.                                                                                                                                                                        |
| 5. Help                | Resolved     | Pass-3 fix retained; E is resolved.                                                                                                                                                         |
| 6. `SaveAll`           | Resolved     | Pass-2 fix retained.                                                                                                                                                                        |
| 7. Mouse wheel         | Resolved     | Pass-3 fix retained.                                                                                                                                                                        |
| 8. Scrollbar           | Resolved     | Pass-2 fix retained.                                                                                                                                                                        |
| 9. Plugin reload       | Resolved     | Pass-2 fix retained.                                                                                                                                                                        |
| A–D                    | Resolved     | Confirmed in pass 3.                                                                                                                                                                        |
| E. Help text and `Esc` | Resolved     | The single-topic buffer contains the runtime page bytes exactly; `Esc` goes through the configured Micro binding. The permanent regression and the original failing review probe both pass. |
| F. Buffer replacement  | Resolved     | Continuation now checks both Micro buffer identities. The permanent regression and the original failing plugin-style replacement probe both pass.                                           |

The E change preserves the synthetic headings only for multiple topics, where
the fixed help overlay must represent multiple Micro help panes in one pane. The
F change invalidates continuation when either buffer is replaced, including a
replacement reached through plugin timer synchronization without a bound action.
The new regressions assert the observed behavior and do not weaken earlier
coverage. The task still has one patch bump (`v0.0.5` to `v0.0.6`) and one
release-note entry.

Pass-4 verification:

- `GOCACHE=/tmp/mdiff-go-pass4-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-pass4-cache go test ./internal/prototype -run 'TestMicroHelpSingleTopicPreservesBytesAndEscBinding|TestDiffNavigationResetsAfterPluginReplacesBuffer' -count=1 -v`
  — passed.
- The two original temporary `go test -overlay` probes for E and F, kept outside
  the repository — both passed.
- `GOCACHE=/tmp/mdiff-go-pass4-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-pass4-cache go build -buildvcs=false -o /tmp/mdiff-pass4-build ./cmd/mdiff`
  — passed.
- `XDG_RUNTIME_DIR=/tmp/mdiff-review-runtime UV_OFFLINE=1 just lint` — blocked
  at `format-check` because this environment denies Docker socket access, as in
  pass 3. `gofmt -l` on the affected Go files and the `uv` Markdown format check
  passed separately. The implementer's final handoff reports `just lint` passed
  in its environment.

## Pass 5 — broader bundled Micro parity audit (2026-10-17)

**Decision: changes required.** Findings 1–9 and A–F stay resolved. The four
findings below are new and open. Each was reproduced with a temporary
`go test -overlay` probe outside the repository; those probes did not change
source or permanent tests.

### G. High — Equal rows lose Micro's search and syntax backgrounds

[`copyLine`](../internal/prototype/render.go) calls `baseCellStyle` for every
captured Micro cell, even when the two lines compare equal. `baseCellStyle`
unconditionally replaces the cell background with the row background. Micro's
renderer deliberately keeps nondefault syntax and `hlsearch` backgrounds above
its cursor-line background
([`bufwindow.go`](../internal/micro/display/bufwindow.go)); it also gives brace
and whitespace highlights their configured styles. This composition strips those
backgrounds on equal lines, where mdiff has no diff reason to recolor anything.
mdiff separately reapplies selection styling.

A probe configured a purple `hlsearch` background, activated search on an equal
line in the left pane, and compared Micro's offscreen rendered cell with mdiff's
final cell. Micro kept purple; mdiff replaced it with the row background. The
README also promises that equal rows use Micro's normal colors.

**Required:** Preserve Micro's complete rendered cell style on equal rows,
including current-line and gutter styling. On changed rows, define and test the
precedence between diff color and Micro's search, selection, syntax, brace, and
whitespace highlights. Add a regression comparing representative Micro-rendered
cells with mdiff's composed cells.

### H. Medium — Large cursor jumps ignore Micro's viewport placement

Micro's [`GotoLoc`](../internal/micro/action/bufpane.go) places a cursor reached
by a distant jump about one quarter of the window height from the top and sets
`StartLine` accordingly.
[`runMicroCommand`](../internal/prototype/app_compat.go) then calls
[`ensureCursorVisible`](../internal/prototype/app.go), which only ensures that
the cursor is somewhere on mdiff's shared viewport. It ignores the new Micro
`StartLine`. This changes ordinary `goto` behavior even with two identical files
and no alignment spacers.

A probe ran `goto 81` in 100 equal lines with an eight-row editor. Micro set its
viewport top to aligned row 77; mdiff displayed row 80 at the top.

**Required:** For a distant jump, map Micro's requested `StartLine` to the
shared aligned viewport before clamping or applying visibility rules. Cover
`goto` and a search result reached through `GotoLoc`, with equal files and
one-sided diff spacers.

### I. Medium — Plugins see the hidden document as the current help pane

Micro's help command opens a `BTHelp` buffer as the active pane
([`command.go`](../internal/micro/action/command.go)), and its Lua `CurPane` API
returns that pane. mdiff displays `helpPane` but leaves the paired document
active in `microTab`; [`luaImportMicro`](../internal/prototype/plugins.go)
always returns `app.focusedPane().h`. A plugin callback handling a help key
therefore sees the help pane as its callback argument while `micro.CurPane()`
and `micro.CurTab().CurPane()` refer to the hidden document. A Lua probe checked
the visible help buffer against `micro.CurPane().Buf` and failed.

**Required:** Make the Micro-facing current-pane APIs refer to visible help
while the overlay is active, without exposing an extra editable document or
breaking the fixed pair. Cover a plugin callback in help that compares its
argument with `CurPane`, and verify the original document becomes current again
when help closes.

### J. High — `QuitAll` from help does not exit the event loop

[`routeHelpAction`](../internal/prototype/app_compat.go) invokes
`requestQuitAll`, which sets `quitComplete` immediately when both documents are
clean. But
[`handleHelpKey` and `handleHelpMouse`](../internal/prototype/app_help.go)
always return `false`, and the help raw-event branch in
[`App.handle`](../internal/prototype/app.go) also returns `false`.
[`RunWithSignals`](../internal/prototype/app.go) exits only when `App.handle`
returns true. A bound `QuitAll` therefore completes but leaves a clean session
running if help is visible. A probe sent the bound key through the event loop:
`quitComplete` became true, then the loop read the closed test stream and
returned `event stream closed` instead of exiting.

**Required:** Propagate completed quit state from help key, mouse, and raw
dispatch into the event-loop return value. Cover bound `QuitAll` from a clean
help overlay and its confirmation path with dirty documents.

Pass-5 verification:

- `GOCACHE=/tmp/mdiff-go-parity-review-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-parity-review-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-parity-review-cache go build -buildvcs=false -o /tmp/mdiff-parity-review-build ./cmd/mdiff`
  — passed.
- Four temporary `go test -overlay` probes for G–J — each failed with the
  observed difference described above.
- `XDG_RUNTIME_DIR=/tmp/mdiff-review-runtime UV_OFFLINE=1 just lint` — blocked
  by denied Docker socket access in `format-check`.

**Decision:** TASK-015 returns to review until G–J are resolved and covered.

## Implementation response to pass 5

All four findings are addressed in the existing TASK-015 change, with no
additional patch version bump:

- **G — addressed:** Equal rows keep Micro's rendered foreground, attributes,
  explicit backgrounds, gutter styling, search, and selection. mdiff's row
  background fills only Micro's ordinary theme or cursor-line background on
  changed rows. On changed characters, the character-diff foreground and
  attributes take precedence, explicit Micro highlight backgrounds remain, and
  selection is applied last. `TestEqualRowCopyPreservesMicroCellStyle` and
  `TestDiffStylePreservesSyntaxForegroundAndCompositionRules` cover the
  composition rules alongside the existing rendered diff palette tests.
- **H — addressed:** Cursor visibility now maps Micro's requested `StartLine`
  into shared aligned rows when a cursor jumps outside the current viewport.
  This preserves Micro's distant-jump placement through equal rows and one-sided
  spacers. `TestLargeGotoAndSearchUseMicroViewportStart` covers `goto`,
  `FindNext`, and both alignment shapes.
- **I — addressed:** While help is visible, its pane temporarily occupies the
  focused slot in Micro's tab pane list. Both `micro.CurPane()` and
  `micro.CurTab():CurPane()` therefore identify the visible help buffer; closing
  help restores the original document pane. The configured plugin fixture checks
  both APIs during a help callback and after closing the overlay.
- **J — addressed:** Help key, mouse, and raw dispatch now return completed quit
  state to the event loop. `TestQuitAllFromHelpExitsEventLoopForKeyMouseAndRaw`
  checks clean `QuitAll` on all three event paths and dirty `QuitAll` after its
  confirmation.

Verification after these changes:

- `GOCACHE=/tmp/mdiff-go-pass5-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-pass5-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-pass5-cache go build -buildvcs=false -o /tmp/mdiff-pass5-build ./cmd/mdiff`
  — passed.

## Pass 6 — independent review of G–J fixes (2026-10-17)

**Decision: changes required.** The fixes for H, I, and J are resolved and
covered. G fixes the original search-background failure, but a theme-specific
case remains open.

| Finding                             | Pass-6 status | Evidence                                                                                                                                    |
| ----------------------------------- | ------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| G. Equal-row Micro highlight styles | **Open**      | The original purple search probe passes, but a search background equal to `cursor-line` is still removed on a noncurrent equal row.         |
| H. Distant cursor jumps             | Resolved      | The original `goto 81` probe passes; the permanent regression covers `goto`, `FindNext`, equal files, and one-sided spacers.                |
| I. Current help pane for plugins    | Resolved      | The original Lua `CurPane()` probe passes; the permanent plugin regression checks both current-pane APIs and restoration after help closes. |
| J. `QuitAll` from help              | Resolved      | The original bound-key probe passes; the permanent regression covers key, mouse, raw, and dirty confirmation.                               |

### Remaining part of G — search highlight collides with `cursor-line`

`baseCellStyle` uses `isCursorLineBackground` to identify a Micro cursor-line
background by comparing only its color value. A Micro search highlight can use
that same color. On a noncurrent equal row, the cell's purple background then
matches the configured `cursor-line` foreground and is replaced with the row's
default background, even though the Micro renderer marked the cell as an
explicit search highlight.

A temporary probe configured both `hlsearch` and `cursor-line` with purple,
placed the cursor on the preceding line, and searched an equal second line.
Micro rendered purple while mdiff rendered the default background. The original
G probe passes because it uses a different `cursor-line` color.

**Required:** Preserve explicit Micro highlight backgrounds even when their
color equals the theme's cursor-line color. Do not infer the origin of a style
from color equality alone. Add a permanent regression for this theme and cursor
placement, while retaining the original search-background and changed-row
composition coverage.

Pass-6 verification:

- `GOCACHE=/tmp/mdiff-go-pass5-cache go test ./...` — passed.
- The original temporary review probes for G–J, combined in one
  `go test -overlay` invocation — all passed.
- The new temporary `go test -overlay` probe for the `hlsearch`/`cursor-line`
  color collision — failed with the remaining behavior described above.
- `GOCACHE=/tmp/mdiff-go-pass5-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-pass5-cache go build -buildvcs=false -o /tmp/mdiff-pass6-build ./cmd/mdiff`
  — passed.
- The reviewer did not rerun `just lint` in this pass; the previous attempt was
  blocked by Docker socket access during `format-check`.

**Decision:** TASK-015 remains in review until the remaining G case is fixed and
covered.

## Implementation response to pass 6

The remaining G case is fixed in the existing TASK-015 change, with no
additional patch version bump. Equal rows now preserve every nondefault
background from Micro's rendered cell without comparing it to the cursor-line
color. The row background supplies only Micro's ordinary theme background, so a
search highlight remains intact even when it uses the same color as the
cursor-line setting.
`TestEqualRowSearchBackgroundSurvivesCursorLineColorCollision` reproduces the
reported theme and cursor placement against the rendered output.

Verification after this change:

- `GOCACHE=/tmp/mdiff-go-pass6-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-pass6-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-pass6-cache go build -buildvcs=false -o /tmp/mdiff-pass6-build ./cmd/mdiff`
  — passed.
- `just lint` — passed.
- `git diff --check` — passed.

## Pass 8 — review of the changed-row background fix (2026-10-17)

**Decision: approved.** The G regression is fixed with explicit background
provenance from Micro's renderer, and the new integration test exercises the
changed-row path that the prior equal-row test missed. Findings 1–9 and A–J are
resolved; no further parity differences outside mdiff's paired-pane and diff
behavior were found in this pass.

The new `BackgroundPreserved` callback reports when Micro preserves an explicit
syntax or search background. mdiff maps that signal back to rendered cells and
uses it when composing row and character-diff styles. This removes the
color-equality guess for highlighted cells while retaining the cursor-line
adaptation for ordinary cell backgrounds.
`TestChangedRowSearchBackgroundSurvivesCursorLineColorCollision` uses differing
files, a search match on the changed row, and colliding `hlsearch` and
`cursor-line` colors. The existing equal-row test still checks the equal-row
path.

Pass-8 verification:

- Focused G–J regressions — passed.
- `GOCACHE=/tmp/mdiff-go-pass7-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-pass7-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-pass7-cache go build -buildvcs=false -o /tmp/mdiff-pass8-build ./cmd/mdiff`
  — passed.
- The user reports `just lint` passed.
- Markdown formatting check — passed.

**Final decision:** TASK-015 meets its acceptance criteria and is approved.

## Pass 7 — follow-up review of pass-6 response (2026-10-17)

**Decision: changes required.** H, I, and J remain resolved. The pass-6 G fix
preserves the reported search background on equal rows, but does not preserve it
on changed rows.

### G remains open — changed-row search highlight collides with `cursor-line`

`baseCellStyle` still calls `isCursorLineBackground` when `different` is true.
If a search highlight's background equals the configured cursor-line color,
mdiff treats that explicit Micro highlight as the ordinary line background and
replaces it with the diff row background. The same color-value inference is also
used by `characterDiffStyle` when composing changed-character styling.

The new permanent regression,
`TestEqualRowSearchBackgroundSurvivesCursorLineColorCollision`, uses identical
files. Its row takes the `different == false` branch, which does not call
`isCursorLineBackground`; therefore it passes without covering the failing path.
A temporary direct probe supplied a Micro purple search style, a purple
`cursor-line`, and a dark-red changed-row style to
`baseCellStyle(..., different: true)`. mdiff returned dark red instead of
purple. This reproduces the remaining regression.

**Required:** Preserve an explicit Micro search or syntax background on a
changed row even when its color equals the cursor-line color. Do not infer style
origin from color equality. Add an integration regression with different files,
a search result on a changed row, and colliding `hlsearch`/`cursor-line` colors;
assert the final rendered cell retains Micro's search background and that
ordinary changed-row backgrounds still compose correctly.

Pass-7 verification:

- `GOCACHE=/tmp/mdiff-go-pass6-cache go test ./...` — passed.
- Existing equal-row collision regression — passed, but does not reach the
  changed-row branch described above.
- Temporary changed-row collision probe — failed: received the diff row
  background instead of the purple search background.
- The user reports `just lint` passed.
- H–J permanent regressions are included in the passing full test suite.

**Decision:** TASK-015 remains in review until the changed-row G case is fixed
and covered.

## Implementation response to pass 7

The changed-row G case is fixed in the existing TASK-015 change, with no
additional patch version bump. Micro's `BufWindow` now reports when a rendered
cell retains an explicit syntax or search background. mdiff carries that
per-cell metadata through offscreen rendering and preserves the background while
applying changed-character foreground styling. Cursor-line background handling
no longer overrides an explicitly preserved Micro highlight, even when both
colors are identical. The new integration regression
`TestChangedRowSearchBackgroundSurvivesCursorLineColorCollision` uses different
files and checks the final rendered search cell. Existing tests continue to
cover ordinary changed-row and character-diff backgrounds.

Verification after this change:

- `GOCACHE=/tmp/mdiff-go-pass7-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-pass7-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `GOCACHE=/tmp/mdiff-go-pass7-cache go build -buildvcs=false -o /tmp/mdiff-pass7-build ./cmd/mdiff`
  — passed.
- `just lint` — passed.
- `git diff --check` — passed.
