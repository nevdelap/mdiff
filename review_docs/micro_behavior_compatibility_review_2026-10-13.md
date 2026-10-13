# Micro behavior compatibility review — 2026-10-13

> Earlier findings and follow-ups are retained as history. The
> [current status](#current-status-of-all-findings) below supersedes their
> snapshot conclusions. General correctness and testing findings stay in the
> separate [project review](project_review_2026-10-13.md).

## What remains to do

**After the first usable release, finish the remaining Micro parity work:**

1. Restore `ShellMode` and `Suspend` through a terminal handoff coordinated with
   mdiff's event reader. It is done when Micro bindings invoke both actions,
   control returns or resumes to the two-pane view, and interaction tests cover
   each path.
2. Finish the blocked-command audit. `raw` opens another tab, `log` and `plugin`
   open a log view, and bundled Micro's `eval` reports unsupported; record those
   reasons. Assess `reload` against the fixed panes and shared viewport. Route
   it through Micro with a regression if safe; otherwise record the exact
   invariant it breaks. The audit is done when every denied built-in command has
   a documented reason or a tested adapter.

**Do not add a second, paired Micro/mdiff executable test matrix.** The current
mdiff interaction tests already exercise the identified key, mouse, command,
raw-event, persistence, and footer cases. A second matrix would duplicate those
checks without closing a demonstrated defect. Do not add a custom equal-row
color or hard-code the illustrative light-theme mockup values; the current
renderer already uses Micro's equal-row colors and theme-based light fallbacks.

The separate [project review](project_review_2026-10-13.md#what-remains-to-do)
lists the remaining source-reuse task. All other behavior findings are done.

## Current status of all findings

This ledger covers the behavior findings, requested color behavior, and
verification requests in this document as of the current post-TASK-010 working
copy. **Done** means the reported difference has an adapter and relevant test.
**To do** names the remaining parity work. **Do not do** closes a historical
request that would duplicate existing coverage or conflict with the agreed color
behavior. Historical findings below retain their original wording and should be
read with this ledger.

| Finding or request                                                                                                    | Status    | Current evidence or remaining work                                                                                                                                                                                                                                       |
| --------------------------------------------------------------------------------------------------------------------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| [Editing keys taken for diff navigation](#p1--existing-editing-keys-are-taken-for-mdiff-navigation)                   | Done      | Micro's default and remapped movement actions reach the focused pane; diff navigation and shared scrolling use bindable actions.                                                                                                                                         |
| [Mouse selection and multicursor behavior](#p1--mouse-events-lose-micro-selection-and-multicursor-behavior)           | Done      | Mouse events map through aligned rows to Micro; interaction tests cover drag, clicks, middle paste, Ctrl-click, and wrapped rows.                                                                                                                                        |
| [Restricted command and command-binding access](#p1--command-access-is-restricted-beyond-pane-safety)                 | To do     | Registered Micro commands, command bindings, and plugin-defined commands are routed through Micro. `safeCommand` still rejects several built-ins; `reload` needs a specific compatibility assessment, while the other blocked view commands need their reasons recorded. |
| [Alternate save, open, and quit bindings](#p1--alternate-save-open-and-quit-bindings-are-disabled)                    | Done      | Configured Micro actions route through focused-pane or two-pane workflows; tests cover command bindings, save/open/quit paths, and footer hints.                                                                                                                         |
| [Raw terminal bindings](#p2--raw-terminal-key-bindings-are-silently-ignored)                                          | Done      | Raw events reach the focused Micro pane; the raw Quit completion bug below was fixed separately.                                                                                                                                                                         |
| [Persistence settings and same-path state](#p2--micro-persistence-settings-are-overridden-globally)                   | Done      | Micro's save settings and prompt history are honored; a same-path regression checks that the focused pane owns saved cursor state.                                                                                                                                       |
| [Footer `statusline` and `keymenu` settings](#p2--status-line-and-key-menu-settings-do-not-control-the-footer)        | Done      | Both settings control the mdiff footer, and a test checks viewport row changes.                                                                                                                                                                                          |
| [Micro help topics](#p3--help-shell-mode-and-config-directory-setup-differ)                                           | Done      | A scrollable overlay shows bundled help and supports topic navigation without replacing either editor buffer.                                                                                                                                                            |
| [Interactive `ShellMode` and `Suspend`](#remaining-explicit-differences)                                              | To do     | Micro actions remain blocked because their terminal handoff is not integrated with mdiff's event reader. This difference is not required by diff alignment.                                                                                                              |
| [Missing config-directory creation](#remaining-explicit-differences)                                                  | Done      | A missing selected default directory is created before Micro reads configuration; a first-run test saves command history there. Explicit `--config-dir` paths must exist.                                                                                                |
| [Raw or mouse Quit not closing mdiff](#p2--raw-quit-completes-internally-but-does-not-close-mdiff)                    | Done      | The event dispatcher reports completed quit state for those event types; the event-loop regression covers raw Quit, ForceQuit, and mouse QuitAll.                                                                                                                        |
| [Help content initially limited to key summary](#p2--help-still-exposes-only-an-mdiff-key-summary)                    | Done      | The bundled help-topic overlay and `help [topic]` command replaced the summary-only behavior.                                                                                                                                                                            |
| [Help Find and repeat](#p3--help-content-is-available-but-micro-help-search-is-still-lost)                            | Done      | Ctrl+F and `n`/`N` search and navigate within the overlay.                                                                                                                                                                                                               |
| [Help match crossing a visual wrap](#p3--help-search-misses-a-word-split-by-visual-wrapping)                          | Done      | Search maps source matches to all occupied display rows; a focused wrap-crossing test covers it.                                                                                                                                                                         |
| [Help regex and `ignorecase`](#p3--help-search-ignores-micros-regex-and-case-setting)                                 | Done      | Help uses Micro's shared regex compiler and case setting; tests cover regex, case sensitivity, and invalid patterns.                                                                                                                                                     |
| [Focused line number, current line, and line cursor](#fifth-independent-re-review)                                    | Done      | The focused Micro painter supplies current-line and line-number styles; mdiff no longer overlays a reversed block cursor.                                                                                                                                                |
| [Configured fake cursor](#p3--configured-fake-cursor-still-bypasses-micros-cursor-policy)                             | Done      | The final frame follows Micro's `fakecursor` setting; a test checks one fake cell and a hidden physical cursor.                                                                                                                                                          |
| [Help-search prompt cursor](#p3--help-search-hides-its-active-prompt-cursor)                                          | Done      | The editor cursor is suppressed only when no InfoBar prompt is active; a test checks both states.                                                                                                                                                                        |
| [Unicode help wrapping and match rows](#p3--help-text-wrapping-still-counts-runes-instead-of-visual-columns)          | Done      | Wrapping and match highlighting share visual-cell mapping; a test covers wide and combining characters.                                                                                                                                                                  |
| [Match on a tab spanning help rows](#p3--a-search-match-on-a-wrapped-tab-highlights-only-its-first-row)               | Done      | Each source rune retains its first and last visual rows; a narrow-width test checks both rows of a wrapped tab.                                                                                                                                                          |
| [Approved dark current/non-current palette](#eighth-visual-color-follow-up)                                           | Done      | Current blue/red/green backgrounds are `#00008B`, `#8B0000`, `#006400`; non-current backgrounds are `#000046`, `#460000`, `#003200`.                                                                                                                                     |
| [Micro-compatible diff color customization](#eighth-visual-color-follow-up)                                           | Done      | When present, `mdiff.diff.*` `color-link` groups in Micro themes override the fallbacks. The light mockup values remain examples, not defaults.                                                                                                                          |
| [Equal rows overriding Micro colors](#p2--equal-rows-override-micros-normal-background)                               | Done      | Equal rows use Micro's ordinary style; focused equal rows use its `cursor-line` style. The `mdiff.diff.equal` group was removed.                                                                                                                                         |
| [Light-theme diff contrast](#p3--dark-diff-fallbacks-also-lack-light-theme-contrast)                                  | Done      | A bundled light-theme test checks pale fallback diff rows and Micro foreground retention; the exact light mockup values were not adopted as defaults.                                                                                                                    |
| [Terminal-default theme treated as dark](#p2--terminal-default-themes-are-treated-as-dark-regardless-of-the-terminal) | Done      | An unknown terminal background stays terminal-default unless a theme explicitly supplies an mdiff row color.                                                                                                                                                             |
| [Coarse-diff visibility on terminal-default themes](#eleventh-follow-up-resolution)                                   | Done      | The center divider marks changed and spacer visual rows with Micro diff foregrounds; the project review covers its regression.                                                                                                                                           |
| [Direct Micro/mdiff behavior matrix](#verification-to-add)                                                            | Do not do | mdiff interaction tests cover the listed key, mouse, command, raw, persistence, and footer paths. A second executable matrix would duplicate these focused tests; exhaustive terminal and plugin coverage is not the release criterion.                                  |

The full Go and race suites, application-package vet, and run-argument check
passed in the [latest re-review](#twelfth-independent-re-review). The open
behavioral differences are the terminal handoff actions and command exclusions
whose necessity has not yet been established.

## Scope and criterion

This is a source-level comparison of mdiff with its bundled Micro
implementation. The criterion is that mdiff should change Micro behavior only
where its purpose requires it: two independently editable file buffers, fixed
side-by-side panes, and one diff-aligned visual viewport. A compatible behavior
may need an mdiff adapter when Micro assumes one pane's private viewport. This
review does not claim exhaustive interactive or terminal coverage, and it does
not change the earlier [project review](project_review_2026-10-13.md).

## Differences required by mdiff's purpose

- Keep exactly two visible editing panes. Micro tab and split actions that
  create or navigate additional views cannot run unchanged. Opening a file or
  starting an unnamed buffer must target one of the two existing panes.
- Align the panes' visual rows, share vertical scrolling, show diff markers, and
  navigate differences. Micro actions that directly change a pane's private
  vertical viewport need an adapter that changes mdiff's shared viewport.
- Route save, open, dirty-buffer, and quit workflows to the correct focused
  buffer or both buffers. The presence of two buffers does not itself require
  removing alternate bindings or ordinary file commands.

The following losses are broader than those constraints. Priority reflects the
effect on a Micro user's existing workflow, not a measured failure rate.

## Findings

### P1 — Existing editing keys are taken for mdiff navigation

Micro maps `AltUp`/`AltDown` to moving lines and `CtrlUp`/`CtrlDown` to moving
the cursor to the buffer start/end in both its
[non-Darwin](../internal/micro/action/defaults_other.go) and
[Darwin](../internal/micro/action/defaults_darwin.go) defaults. mdiff intercepts
those physical keys before the focused Micro pane receives them
([event dispatcher](../internal/prototype/app.go)). They instead jump between
diffs or scroll the shared viewport, even if the user remapped them. Diff
navigation and shared scrolling are required; replacing these particular Micro
actions is an avoidable compatibility choice.

**Recommendation:** Preserve Micro's bound action for these keys. Give mdiff
navigation its own configurable bindings and resolve conflicts explicitly. Test
both default actions and user remaps through mdiff's event dispatcher.

### P1 — Mouse events lose Micro selection and multicursor behavior

mdiff consumes every mouse event in
[`App.handleMouse`](../internal/prototype/app.go), which handles wheel movement
and left-button cursor placement. Micro's
[`BufPane.HandleEvent`](../internal/micro/action/bufpane.go) handles press,
drag, and release, and its
[default bindings](../internal/micro/action/defaults_other.go) include
middle-click paste and Ctrl-click multicursor. Micro also implements drag
selection and double/triple-click selection in its buffer actions. Those paths
are unreachable in mdiff. Translating a screen position through an aligned or
wrapped row is required; discarding the mouse action after translation is not.

**Recommendation:** Translate coordinates to the focused pane, then dispatch
mouse press, drag, release, and modifier actions through Micro. Cover ordinary
rows, wrapped rows, and alignment spacers in interaction tests.

### P1 — Command access is restricted beyond pane safety

mdiff's [command bar](../internal/prototype/app.go) accepts only `goto`, `jump`,
`replace`, `replaceall`, `retab`, `show`, `showkey`, and `memusage`. Micro's
[command registry](../internal/micro/action/command.go) also has `set`,
`setlocal`, `toggle`, `bind`, `unbind`, `pwd`, `cd`, `textfilter`, and
plugin-defined commands, among others. Many do not create a third view or alter
the shared viewport. Moreover,
[`BufMapEvent`](../internal/micro/action/bufpane.go) rejects every `command:`
and `command-edit:` binding; this disables Micro's default `Ctrl+L` goto prompt.

**Recommendation:** Classify commands and binding forms by actual effects. Adapt
commands that target the focused buffer, shared viewport, or two-buffer file
workflow. Reject only operations that cannot satisfy the two-pane model, and
explain those rejections.

### P1 — Alternate save, open, and quit bindings are disabled

[`prototypeUnsafeAction`](../internal/micro/action/bufpane.go) rejects `Save`,
`SaveAll`, `SaveAs`, `OpenFile`, `Quit`, and related actions regardless of their
bound key. mdiff instead hardcodes `Ctrl+O`, `Ctrl+S`, and `Ctrl+Q` in
[`App.handle`](../internal/prototype/app.go). Micro's default F2 save and F4/F10
quit therefore stop working, as do user key remaps. Saving the focused named
file under another name or saving both buffers can be defined without adding a
pane. The two-buffer model requires careful routing, not blanket removal.

**Recommendation:** Route compatible Micro actions through mdiff's focused pane
and dirty-buffer workflow; define sequential or prompted `SaveAll` semantics.
Verify default alternate keys, remaps, and the footer hints. The footer
currently reads configured bindings from
[`config.Bindings`](../internal/prototype/app_render.go) even though some of
those bindings are rejected, so misleading hints are possible; confirm that case
with an interaction test.

### P2 — Raw terminal key bindings are silently ignored

Micro's [pane event handler](../internal/micro/action/bufpane.go) routes
`tcell.EventRaw` into raw escape-sequence bindings. mdiff's
[`App.handle`](../internal/prototype/app.go) has cases only for resize, mouse,
key, and paste events. The fixed two-pane layout does not require dropping raw
bindings. Forward raw events to the focused pane, subject to the same
action-safety routing as other bindings.

### P2 — Micro persistence settings are overridden globally

[`NewWithFiles`](../internal/prototype/app.go) sets
`DisableBufferStateWrites = true` and `savehistory = false` after loading the
user's settings. Micro
[loads saved buffer state](../internal/micro/buffer/buffer.go) when `savecursor`
or `saveundo` is enabled, but the global switch prevents writing it back on
close. The source comment for disabling history says mdiff does not expose
Micro's command bar, while mdiff exposes a command prompt on `Ctrl+E`. These are
visible departures from configured Micro behavior.

**Recommendation:** Honor the user's persistence choices, with mdiff-specific
handling for two buffers that may refer to the same file. Decide deliberately
which prompt histories can be reused, and update the comment and documentation
to match the resulting behavior. State collisions for the same path need an
explicit rule before simply enabling writes.

### P2 — Status line and key menu settings do not control the footer

Micro defaults `keymenu` to false and `statusline` to true
([settings](../internal/micro/config/settings.go)). mdiff
[always draws](../internal/prototype/app_render.go) two status lines and two
menu rows when there is room, while the Micro `ToggleKeyMenu` action is
rejected. A two-pane diff needs some way to identify panes and present status,
but fixed menu visibility and ignoring these settings are separate product
choices.

**Recommendation:** Apply the settings to an mdiff-aware footer, preserving pane
identity and diagnostics. Keep the aligned viewport's available row count in
sync when the menu changes.

### P3 — Help, shell mode, and config-directory setup differ

mdiff's `Ctrl+G` shows a one-line key summary where Micro's `ToggleHelp` opens
help content; the Micro action is also rejected
([mdiff dispatcher](../internal/prototype/app.go),
[Micro action filter](../internal/micro/action/bufpane.go)). The filter also
rejects `ShellMode`. Both features need integration with mdiff's fixed panes and
terminal lifecycle, but their complete removal is not established as necessary.
mdiff's [config-directory setup](../internal/prototype/app.go) also follows a
documented no-create policy, whereas
[Micro creates its default config directory](../internal/micro/config/config.go).
That policy is a deliberate extra behavior difference, not a consequence of diff
alignment.

**Recommendation:** Offer help within the two-pane UI and assess shell-mode
restoration around the terminal. Keep the no-create config policy only if it is
an explicit mdiff product requirement; document its compatibility cost.

## Verification to add

Create a small behavior matrix comparing bundled Micro and mdiff through actual
events, not just individual action functions. At minimum cover default and
remapped movement/save/open/quit keys; mouse click, drag, multi-click,
middle-click, and Ctrl-click; allowed and blocked commands; raw bindings;
configured persistence; and `keymenu`/`statusline` changes. Test one file per
pane and the same file in both panes. For each intentional difference, record
the two-pane invariant that requires it.

This review is based on dispatch and configuration paths in the current
checkout. Runtime feasibility of adapted help, shell mode, and same-path state
writing remains to be established by implementation and interactive tests.

## Implementation follow-up

The behavior compatibility change routes configured Micro actions through
mdiff's two-pane adapters. Default Alt-Up/Down and Ctrl-Up/Down reach Micro;
`DiffPrevious`/`DiffNext` and scroll actions are bindable and use the aligned
viewport. Mouse press, drag, release, middle paste, and Ctrl-click reach Micro
after aligned-row mapping. Raw events also reach the focused pane.

Save, Save As, Save All, Open, Quit, and New File use the focused pane or a
sequential two-pane workflow. Micro's safe setting, binding, navigation, file,
and text commands, plus plugin-defined commands, are available through both the
command bar and command bindings. Commands that create extra views or require a
terminal handoff remain unavailable. The footer only advertises live `keymenu`
and `statusline`. Buffer state and prompt history follow Micro's settings; for
duplicate paths the focused pane owns the saved state.

Help is shown as a temporary overlay above the two editor panes. Shell mode
remains unavailable: Micro's `TempFini`/`TempStart` terminal handoff does not
coordinate with mdiff's independent event reader. The no-create config directory
policy remains an explicit mdiff choice, documented in README.

## Independent re-review of the current checkout

The new adapter and tests resolve most of the original compatibility findings.
Micro's default Alt-Up/Down and Ctrl-Up/Down actions now reach the focused
buffer, while diff and shared-scroll actions are bindable. Mouse selection,
multi-click, middle paste, Ctrl-click cursors, and raw bindings are dispatched
through Micro with aligned-row mapping. Save, open, quit, command bindings,
history, buffer state, and the configurable footer have corresponding
application-level tests. The same-path state rule is explicit.

### P2 — Raw Quit completes internally but does not close mdiff

[`routeMicroAction`](../internal/prototype/app_compat.go) can complete `Quit`,
`QuitAll`, or `ForceQuit` for a raw escape-sequence binding.
[`App.handle`](../internal/prototype/app.go) returns `a.quitComplete` from its
key-event branch, but its raw-event branch falls through to `return false`.
`RunWithSignals` therefore redraws and waits for another event even though the
quit workflow completed. Mouse-bound quit actions have the same dispatcher shape
and need checking.

**Reproduction:** a temporary test bound a raw sequence to `Quit`, sent that
`EventRaw`, and observed `app.quitComplete == true` with
`app.handle(event) == false`. The test failed as expected and was removed.

**Fix:** return the completed quit state for every event type that can execute a
Micro action, or centralize the exit check after dispatch. Add a regression
through `RunWithSignals` for raw Quit and relevant mouse bindings.

### P2 — Help still exposes only an mdiff key summary

`ToggleHelp` now opens a short
[`helpVisible` overlay](../internal/prototype/app_render.go), but it does not
show Micro's bundled help pages or topic navigation.
[`safeCommand`](../internal/prototype/app.go) still rejects Micro's `help`
command, though the [Micro command](../internal/micro/action/command.go) is
available in the bundled source. Showing a help document inside the fixed
two-pane UI requires an adapter; removing help topics is not required by diff
alignment. Add an overlay or focused-pane help mode that can display requested
topics and return without changing the two-buffer layout.

### Remaining explicit differences

- `ShellMode` and `Suspend` remain unavailable. The current implementation lacks
  a terminal handoff compatible with mdiff's independent event reader. This is
  an integration limit rather than a diff-view requirement; preserve the
  documented limitation until the handoff is implemented and tested.
- mdiff still does not create a missing default config directory, whereas Micro
  does. README documents this policy, but it is not required by the fixed-pane
  or aligned-viewport purpose. Keep it only as an explicit product exception to
  Micro compatibility.

The prompt-focus buffer desynchronization found during this pass is documented
in the
[project re-review](project_review_2026-10-13.md#re-review-of-the-current-checkout)
because it is an application correctness defect in the new command routing. The
temporary probes were removed; no implementation or test source was changed by
this review.

### Follow-up resolution

The event dispatcher now returns completed quit state after raw and mouse
actions. `TestRawAndMouseQuitExitEventLoop` verifies raw Quit, raw ForceQuit,
and mouse QuitAll through `RunWithSignals`. Micro's bundled help topics now
appear in a scrollable overlay; `help [topic]` and left/right topic navigation
work without replacing either editor buffer.
`TestMicroHelpTopicsUseOverlayWithoutReplacingBuffers` checks rendering,
scrolling, navigation, and the two-pane layout. ShellMode, Suspend, and the
explicit config-directory policy remain documented limits.

## Second independent re-review

The raw and mouse Quit defect is fixed: the dispatcher now returns the completed
quit state for those events, and
[`TestRawAndMouseQuitExitEventLoop`](../internal/prototype/behavior_compat_test.go)
exercises the actual event loop. Bundled Micro help topics are now readable in
an overlay, with scrolling and topic navigation, and the two editor buffers
remain intact. These changes close the two new findings from the preceding
re-review.

### P3 — Help content is available, but Micro help search is still lost

Micro opens help content in a `BufPane`, where its normal Find action can search
the help text. mdiff's [help key handler](../internal/prototype/app_help.go)
handles scrolling and topic changes but closes the overlay on any other key. For
example, `Ctrl+F` is consumed while help is visible, so a user cannot search a
long help topic. The two-pane layout requires a different presentation of help,
but does not require losing help search. An overlay-specific Find action, or an
adapted read-only help view, would preserve that workflow.

### Remaining exceptions

`ShellMode` and `Suspend` remain unavailable because the current terminal
handoff is not integrated with mdiff's event reader. The no-create config
directory policy also remains an explicit product choice. Neither difference
follows from diff alignment itself; the existing documentation should continue
to identify them until they are resolved or accepted as permanent exceptions.

A newly reproduced prompt-resize defect is recorded in the separate
[project review](project_review_2026-10-13.md#second-re-review-of-the-current-checkout).
It stems from stale Micro pane references during resize and is a correctness
issue in the two-pane application, rather than a separate Micro feature policy.

### Second follow-up resolution

The help overlay now accepts Ctrl+F, searches the current bundled topic,
highlights the selected matching row, and moves between matches with `n` and
`N`. `TestMicroHelpFindAndRepeatKeepsOverlay` checks prompt entry, navigation,
rendering, and preservation of the two editor buffers. ShellMode, Suspend, and
the explicit no-create config-directory policy remain documented exceptions.

## Third independent re-review

The help overlay now supports ordinary `Ctrl+F` searches and `n`/`N` match
navigation, resolving the previous broad help-search gap. The current working
copy remains the single post-TASK-010 change with one version bump; the Micro
behavior assessment below applies to this in-progress snapshot.

### P3 — Help search misses a word split by visual wrapping

[Help search](../internal/prototype/app_help.go) scans the already wrapped
display rows. If the terminal width splits a contiguous word or phrase across
two rows, no row contains the full query and mdiff reports no match. For
example, at width 12, searching `longneedle` in `before longneedle after`
returns no result. Micro searches the underlying help buffer text, so the
two-pane help overlay need not lose this behavior. Search source lines, then map
each match to the wrapped display row for navigation and highlighting. Add a
narrow-terminal regression test for a match crossing a wrap boundary.

`ShellMode`, `Suspend`, and the explicit no-create config-directory policy
remain the documented compatibility exceptions. The stale README key and Save As
descriptions are recorded in the separate
[project review](project_review_2026-10-13.md#third-re-review-of-the-current-checkout)
because they are documentation errors, not behavioral differences in the
application.

### Third follow-up resolution

Help search now scans each unwrapped source line and maps match rune offsets to
display rows. A match crossing a visual wrap highlights both rows and remains
navigable with `n` and `N`. `TestMicroHelpFindCrossesVisualWrap` checks the
narrow-terminal case with `longneedle` split between two rows. The existing
ShellMode, Suspend, and config-directory exceptions remain documented.

## Fourth independent re-review

The visual-wrap help-search finding is resolved, including a regression test for
a match that crosses two display rows. I found one further help-search
difference in the current overlay. The earlier default-key, mouse, command,
save, persistence, and footer compatibility fixes remain in place.

### P3 — Help search ignores Micro's regex and case setting

Micro's `Ctrl+F` [Find action](../internal/micro/action/actions.go) searches as
a regular expression by default, and its
[buffer search](../internal/micro/buffer/search.go) honors `ignorecase`. The
[mdiff help overlay](../internal/prototype/app_help.go) instead compares each
fixed-length rune slice with `strings.EqualFold`. It always ignores case and
always treats regex punctuation literally. A temporary, removed probe confirmed
that `a.b` misses `axb` in the help overlay and lowercase `alpha` matches
`Alpha`; the latter result cannot be changed with Micro's `ignorecase=false`
setting because the overlay does not read that setting. Both differences are
avoidable in the two-pane help presentation. Apply Micro's search semantics to
the unwrapped help source, map returned ranges to display rows, and test a regex
match plus `ignorecase=false`. Keep the existing wrap-crossing regression.

`ShellMode`, `Suspend`, and the no-create config-directory policy remain the
documented exceptions. No other new compatibility difference was identified in
this pass.

### Fourth follow-up resolution

The help overlay now compiles the search query as a Go regular expression,
applies Micro's global `ignorecase` setting, and maps byte match ranges through
rune positions to the wrapped display rows. Invalid regexes report an error and
preserve the previous search. `TestMicroHelpFindUsesRegexAndIgnorecaseSetting`
checks regex punctuation, case-sensitive matching, and invalid patterns; the
visual-wrap regression remains in place. The documented ShellMode, Suspend, and
config-directory exceptions remain unchanged.

## Fifth independent re-review

Igor's regex and `ignorecase` help-search fix resolves the fourth pass's
finding. During this review I extracted the search-pattern compiler into Micro's
buffer package; `FindNext` and mdiff's help overlay now use it together. The
wrap-crossing, case-setting, regex, and invalid-pattern tests pass.

The renderer had painted Micro's focused pane as inactive, losing its
current-line color and current-line-number style, then reversed the cursor cell
into a block. Changes made during this review, retained at the user's request,
now paint the focused pane as active and remove that extra block. Ordinary diff
rows retain dark red, blue, and green backgrounds; a focused current diff line
uses the corresponding lighter color. Equal current lines use Micro's configured
`cursor-line` color, and focused line numbers retain Micro's
`current-line-number` foreground. Focused rendering tests, the full Go suite,
and the race suite pass. The terminal's physical cursor supplies its visible
shape, as in Micro.

### P3 — Configured fake cursor still bypasses Micro's cursor policy

Micro's [screen.ShowCursor](../internal/micro/screen/screen.go) chooses a fake
cursor when `fakecursor` is enabled. mdiff's
[final draw](../internal/prototype/app_render.go) calls the terminal's
`ShowCursor` directly, so the physical cursor remains visible regardless of that
setting. With the focused Micro painter active, its offscreen draw can also
reverse a cursor cell before mdiff copies it, leaving two cursor marks. This is
a source-level finding; the current tests cover the default physical cursor but
do not exercise `fakecursor=true`. Route final cursor presentation through one
Micro-compatible policy and add a setting-specific rendering test.

`ShellMode`, `Suspend`, and the no-create config-directory policy remain the
documented exceptions. The wide-character prompt defect and other Micro-code
reuse opportunities are recorded in the separate
[project review](project_review_2026-10-13.md#fifth-re-review-and-micro-reuse-audit).

### Fifth follow-up resolution

The focused Micro painter now suppresses cursor presentation only while
composing a pane offscreen, preserving its current-line and line-number styles.
After copying the frame, mdiff follows Micro's `fakecursor` setting: a real
terminal cursor for the default setting, or one reversed cell with the terminal
cursor hidden. `TestFakeCursorUsesOneTerminalCellAndNoPhysicalCursor` checks
both the offscreen frame and the final terminal frame, including a redraw. The
shared prompt-row renderer also fixes wide-character cursor placement as
described in the project review.

## Sixth independent re-review

Igor's latest renderer changes preserve Micro's active-line styling, line number
foreground, and configured fake-cursor behavior. The prompt now uses Micro's
`InfoWindow` visual-column calculations. The full Go and race suites,
application-package vet, and run-argument check pass. This pass changed review
documents only.

### P3 — Help search hides its active prompt cursor

The help overlay keeps `helpVisible=true` while
[`openHelpSearch`](../internal/prototype/app_help.go) opens Micro's InfoBar
prompt. [`draw`](../internal/prototype/app_render.go) paints that prompt and
computes its cursor position, then unconditionally sets `cursorVisible=false`
whenever help is visible. Thus `Ctrl+F` within help shows typed text without the
cursor that Micro normally shows in a Find prompt. Suppress the editing cursor
only while the help overlay has no active prompt. Add a focused test for cursor
visibility during help search and after the prompt closes.

### P3 — Help text wrapping still counts runes instead of visual columns

[`helpLines` and `helpMatches`](../internal/prototype/app_help.go) wrap at
`len([]rune(line))` or `utf8.RuneCountInString`, while the overlay's
[`renderLine`](../internal/prototype/app_render.go) now paints by terminal-cell
width. For a help line containing a wide character or combining sequence, the
calculated row boundaries can differ from what is painted; text or match
highlighting may land on the wrong display row. The bundled English help pages
appear ASCII, so this concerns Unicode or user-provided help content. Use the
same visual-width mapping for wrapping and match positions, with a narrow-width
Unicode help test. Micro's normal buffer display already accounts for visual
width, so the two-pane help overlay does not require this difference.

The previously documented `ShellMode`, `Suspend`, and config-directory setup
exceptions remain. The two-row prompt issue is recorded in the separate
[project review](project_review_2026-10-13.md#sixth-re-review-of-the-current-checkout).

### Sixth follow-up resolution

The help overlay now hides the editor cursor only when no InfoBar prompt is
active. `TestHelpSearchPromptShowsCursorUntilClosed` checks that the Find prompt
cursor appears while typing and disappears when the prompt closes without
closing help.

Help wrapping and regex-match row mapping now use one visual-cell mapping. Wide
characters and combining marks stay together, and the display and match
highlight agree on wrapped rows.
`TestUnicodeHelpWrapAndMatchRowsFollowVisualColumns` checks a match crossing a
wide-character boundary into a row with a combining sequence. The two-row prompt
fix is recorded in the project review.

## Seventh independent re-review

Igor's sixth follow-up fixes the visible help-search cursor and maps wide and
combining characters to the rows that the overlay paints. Focused tests cover
both cases; the full Go and race suites pass. The remaining difference below is
confined to help content with a tab that spans a visual row boundary.

### P3 — A search match on a wrapped tab highlights only its first row

[`wrapHelpLine`](../internal/prototype/app_help.go) expands a tab into spaces
and may put those spaces on two display rows, but appends only the tab's initial
row to `runeRows`. [`helpMatches`](../internal/prototype/app_help.go) uses that
single row for a regex match on the tab. For example, at width three, a tab at
column zero paints three spaces on one row and one on the next; searching for
`\t` highlights only the first row. This is a source-level finding; the Unicode
regression covers wide and combining characters, not a tab split across rows.
Record every row occupied by a matched tab, or keep a start/end display-row
range per source character. Add a narrow-width tab search test. The bundled help
pages do not appear to contain tabs, but Micro can load plugin or user help
content.

The previously documented `ShellMode`, `Suspend`, and config-directory setup
exceptions remain. No other new Micro behavior difference was found in this
pass.

## Eighth visual color follow-up

The palette requirement is now more specific: use the **existing non-current**
blue, red, and green backgrounds as the **current-line** backgrounds. Give
non-current diff rows corresponding backgrounds at about half that brightness.
The reviewed code currently uses tcell's `ColorDarkBlue`, `ColorDarkRed`, and
`ColorDarkGreen` for non-current rows, and brighter `ColorBlue`, `ColorRed`, and
`ColorGreen` for current rows. Thus the former dark colors move to current rows;
the brighter colors cease to be the current-line defaults. Here,
"half-brightness" is interpreted as a request about the diff **backgrounds**,
not the foreground text. Keep Micro's syntax and selection foregrounds,
configured `current-line-number` style, and line-shaped cursor. Equal current
rows continue to use Micro's theme `cursor-line` color. The earlier dark-neutral
proposal for equal non-current rows is superseded by the user's later direction
in the [ninth re-review](#ninth-independent-re-review).

The user approved an HTML preview of these exact fallback pairs:

| Diff row         | Current background | Non-current background |
| ---------------- | ------------------ | ---------------------- |
| Changed blue     | `#00008B`          | `#000046`              |
| Left-only red    | `#8B0000`          | `#460000`              |
| Right-only green | `#006400`          | `#003200`              |

The non-current hex values approximately halve the corresponding RGB channel. No
equal-row color was part of that preview.

The HTML mdiff mockup also shows these **illustrative recommendations for a
light Micro theme**. They have not been approved as project defaults:

| Diff row         | Current background | Non-current background |
| ---------------- | ------------------ | ---------------------- |
| Changed blue     | `#B6D6FF`          | `#E7F1FF`              |
| Left-only red    | `#F2B5B5`          | `#FBE7E7`              |
| Right-only green | `#B8E4C0`          | `#E5F4E9`              |

On a light base, the non-current backgrounds are paler and the current-line
backgrounds are more saturated. A light theme can set these values through the
same optional `color-link` groups; its equal current line still follows that
theme's `cursor-line` color.

The colors should be customizable through Micro's existing `.micro` colorscheme
format. [`ParseColorscheme`](../internal/micro/config/colorscheme.go) accepts
arbitrary `color-link` group names and stores them in `config.Colorscheme`;
mdiff already loads the selected Micro theme. Add optional namespaced groups for
the three diff kinds in both current and non-current states. For example,
`color-link mdiff.diff.changed.current ",#00008B"` and
`color-link mdiff.diff.changed ",#000046"` can live in a Micro theme file. Read
only the groups mdiff needs and retain built-in fallbacks when a theme omits
them. This keeps existing Micro themes valid and avoids a second theme parser or
separate settings format. Do not reuse Micro's `diff-added`, `diff-modified`,
and `diff-deleted` foreground styles as row backgrounds.

[`renderPane` and `diffStyle`](../internal/prototype/render.go) currently
hard-code the diff backgrounds. Update
[`TestFocusedLineKeepsMicroLineNumberAndLighterDiffBackground`](../internal/prototype/render_test.go)
to check the new current/non-current relationship and add a renderer check that
custom `color-link` groups override the defaults without replacing Micro's
foreground styles. This review records the approved diff palette; no renderer
change is made here.

### Seventh and eighth follow-up resolution

Help search now keeps a first/last visual-row range for each source rune.
Expanded tab spaces that cross a wrap contribute both rows to the range;
`TestHelpSearchHighlightsEveryRowOfWrappedTab` checks a width-three terminal and
a regex match on the tab. The earlier wide and combining-character help tests
still pass.

The built-in current-line diff backgrounds are the approved dark blue `#00008B`,
red `#8B0000`, and green `#006400`. Their non-current counterparts are
`#000046`, `#460000`, and `#003200`. Equal non-current rows use a chosen
dark-neutral `#181818` fallback; equal current rows still use Micro's
`cursor-line` color. Each diff row background can be overridden through the
documented `mdiff.diff.*` `color-link` groups in a Micro `.micro` colorscheme.
`TestFocusedLineKeepsMicroLineNumberAndApprovedDiffPalette` checks the fallback
relationship, and
`TestMicroColorschemeOverridesDiffBackgroundsWithoutReplacingForegrounds` checks
parsed theme overrides and retention of Micro's changed-character and
current-line-number foregrounds. The light-theme examples above remain optional
examples, not built-in defaults.

## Ninth independent re-review

The seventh pass's wrapped-tab search finding is resolved. The requested dark
palette now uses the approved current/non-current pairs, and mdiff reads
optional `mdiff.diff.*` groups from Micro colorschemes. Focused tests, the full
Go suite, and the race suite pass. One compatibility problem remains in the
fallback behavior for existing light Micro themes.

### P2 — Equal rows override Micro's normal background

[`renderPane`](../internal/prototype/render.go) now paints every non-current
equal row with `#181818` when `mdiff.diff.equal` is absent, while
[`baseCellStyle`](../internal/prototype/render.go) retains Micro's text
foreground. Bundled light themes such as
[`dukelight-tc`](../internal/micro/runtime/colorschemes/dukelight-tc.micro)
define black text on `#F0F0F0` and contain no `mdiff.diff.*` groups. Selecting
that unmodified Micro theme therefore puts black text on `#181818` for most
equal lines. The user has clarified the intended behavior: equal rows on both
sides should use Micro's normal colors, with Micro's `cursor-line` style on the
focused current line. Remove the `mdiff.diff.equal` group and its `#181818`
fallback from the renderer and README. Use `config.DefStyle` for non-current
equal rows, leaving syntax and line-number styles with Micro. Add a test that
loads an existing light Micro theme without mdiff-specific entries and checks
equal text and row backgrounds. This behavior difference is not required by the
two-pane layout.

### P3 — Dark diff fallbacks also lack light-theme contrast

The approved red, blue, and green defaults are dark and are applied under every
Micro theme unless that theme supplies the optional `mdiff.diff.*` groups.
Existing bundled light themes do not supply those groups, so their dark text can
have weak contrast on changed and spacer rows. The new theme test supplies all
mdiff groups and does not cover this default path. Keep the approved dark
palette for dark themes, and provide readable behavior for unmodified light
themes while retaining explicit `color-link` overrides. The light mockup values
are examples, not approved defaults. Include changed and spacer rows in the
light-theme render test.

### Ninth follow-up resolution

Non-current equal rows now use `config.DefStyle` directly, retaining Micro's
normal foreground and background on both sides. Focused equal rows still use
Micro's `cursor-line` background. The `mdiff.diff.equal` override and `#181818`
fallback have been removed from the renderer and README.

For light Micro themes without mdiff color groups, changed and spacer row
backgrounds now blend a blue, red, or green accent into the theme's normal
background. Current rows use a stronger blend. Dark themes keep the approved
dark palette, and explicit `mdiff.diff.*` groups still take precedence. The
bundled `dukelight-tc` renderer test covers equal rows, changed characters,
spacer backgrounds, the current line number, and a light-theme override.

## Tenth independent re-review

The ninth pass's equal-row and explicit-light-theme findings are resolved. Equal
rows now use Micro's regular colors, and a light theme with a known RGB
background receives readable, pale diff rows. The full Go and race suites pass.
One Micro theme case remains outside that fallback and test.

### P2 — Terminal-default themes are treated as dark regardless of the terminal

[`lightThemeBackground`](../internal/prototype/render.go) returns false when
Micro's default background has no RGB value. Bundled 16-color themes such as
[`simple`](../internal/micro/runtime/colorschemes/simple.micro) leave the
default background to the terminal, so mdiff then applies its dark blue, red,
and green backgrounds even when that terminal has a light background and dark
text. Micro can display ordinary text using the terminal's own colors, while
mdiff's changed and spacer rows may become difficult to read. The new
`dukelight-tc` regression has an explicit `#F0F0F0` background and does not
exercise this case. The approved dark palette still works for themes with a
known dark background.

When Micro's background is terminal-default, avoid assuming a dark terminal. One
compatible fallback is to retain the terminal background on diff rows and use
existing diff markers and Micro's changed-character styles until explicit
`mdiff.diff.*` groups provide row colors. Keep theme overrides authoritative.
Add a focused test with the bundled `simple` colorscheme and an unset default
background; it should verify the chosen fallback and preserve Micro's text
styles. A terminal-default theme may be used on either a light or dark terminal,
so RGB-based theme detection alone cannot settle its contrast.

### Tenth follow-up resolution

Diff rows now use the terminal background when `config.DefStyle` has no
resolvable RGB background. This avoids imposing the dark palette on an unknown
terminal while Micro's changed-character foregrounds and line-number styles
remain in effect. Spacer rows remain blank alignment rows. A theme can supply
explicit `mdiff.diff.*` groups to color any diff row, and those groups take
precedence over this fallback. Known dark backgrounds still use the approved
dark palette; known light backgrounds still use the pale blended colors.

`TestBundledTerminalDefaultThemeKeepsTerminalBackground` loads the bundled
`simple` scheme unchanged, checks equal, changed, and spacer rows plus Micro's
foreground styles, then verifies that explicit row-color overrides work.

## Eleventh independent re-review

The terminal-default theme finding is resolved: mdiff retains the terminal's
background when Micro leaves it unset, and optional `mdiff.diff.*` colors still
take precedence. Equal rows continue to use Micro's normal colors, with its
configured current-line and line-number styles. The full Go and race suites
pass. I found no additional Micro-specific behavior difference in this pass.

The tenth pass suggested using "existing diff markers" with terminal-default
backgrounds, but the renderer currently has no whole-row diff gutter marker.
That correction and the resulting coarse-diff visibility finding are recorded in
the separate
[project review](project_review_2026-10-13.md#tenth-re-review-of-the-current-checkout).

### Eleventh follow-up resolution

The center divider now supplies a visible cue for changed and spacer rows when
Micro leaves its background to the terminal. Its `!`, `<`, and `>` markers use
Micro's existing diff foreground styles while both pane backgrounds stay at the
terminal default. Equal rows retain the ordinary divider. The focused coarse
change and spacer test covers the case without changed-character spans; the
project review records the full resolution.

## Twelfth independent re-review

The new divider markers serve mdiff's required diff-aligned view without
changing Micro's text colors, line-number colors, or equal-row rendering. They
reuse Micro's `diff-modified`, `diff-deleted`, and `diff-added` foreground
styles when its theme leaves the background to the terminal. The marker is
selected from the shared visual row, so wrapped changed rows and spacer
continuations stay identifiable. I found no new behavioral difference beyond
mdiff's stated purpose in this pass. The full Go and race suites,
application-package vet, and run-argument check pass. The general review records
the resolved coarse-diff finding separately.

## Pre-merge config-directory resolution

mdiff now creates the selected default Micro config directory when it is
missing, following the `MICRO_CONFIG_HOME`, `XDG_CONFIG_HOME/micro`, then
`~/.config/micro` precedence. The directory exists before Micro reads settings
or plugins, and a first-run test confirms command history can be saved there.
Missing explicit `--config-dir` paths still fail clearly. No default settings or
bindings files are written merely by creating the directory. This closes the
first-release config-directory item in the current-status ledger above.
