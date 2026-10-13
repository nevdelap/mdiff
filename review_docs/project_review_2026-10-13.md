# Project review — 2026-10-13

> Earlier findings and follow-ups are retained as history. The
> [current status](#current-status-of-all-findings) below supersedes their
> snapshot conclusions. Micro behavior differences remain in the separate
> [compatibility review](micro_behavior_compatibility_review_2026-10-13.md).

## What remains to do

**No project finding blocks a first usable release.** After that release, move
`sourceRuneLocations`' visual-position calculation into a shared Micro API and
call it from mdiff. Keep its current linear work per line. This definite
maintenance task is done when the duplicate calculation is removed and the
Unicode, tab, and soft-wrap rendering tests pass.

**Do not do the proposed package rename, further `app.go` splitting, or
options-based constructor redesign solely for this review.** The disposable
comment has been removed and the major responsibilities are already split into
files. The remaining names and constructor wrappers have no identified behavior
failure or demonstrated duplication that justifies that churn.

The
[Micro compatibility review](micro_behavior_compatibility_review_2026-10-13.md#what-remains-to-do)
lists the behavior work still to do. Exhaustive testing of every left/right
input, terminal, and plugin is not a completion criterion.

## Current status of all findings

This ledger covers every finding and simplification request in this document as
of the current post-TASK-010 working copy. **Done** means the reported issue is
fixed or the required check exists. **To do** names remaining work. **Do not
do** closes a historical suggestion that is not justified by the current code.
Earlier reproductions, coverage figures, and conclusions below describe the
checkout at the time of each pass; they are not claims about today's code.

| Finding or request                                                                                                           | Status    | Current evidence or remaining work                                                                                                                                                       |
| ---------------------------------------------------------------------------------------------------------------------------- | --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Initial file-backed diff](#p1--file-backed-startup-never-builds-the-initial-diff)                                           | Done      | `NewWithFiles` builds the first layout from the supplied files; the first-draw test covers multiline, insertion, unchanged, and one-file cases.                                          |
| [Left/right diff test breadth](#p2--line-level-diff-correctness-has-limited-input-breadth)                                   | Done      | Explicit alignment cases, 500 generated pairs, an independent LCS check on 100 unique-line cases, span reconstruction, and seven file-backed first-draw cases cover the requested paths. |
| [`just run` argument forwarding](#p2--documented-just-run-invocation-fails-flags-are-swallowed)                              | Done      | The recipe forwards `"$@"`; the script and CI check two files, a flag, and a spaced path.                                                                                                |
| [Race and fuzz CI](#p2--race-and-fuzz-checks-exist-but-are-absent-from-the-ci-gate)                                          | Done      | CI runs a race job and a bounded comparison fuzz job on the weekly schedule or manual dispatch.                                                                                          |
| [Coverage interpretation](#p3--coverage-reports-need-an-explicit-interpretation)                                             | Done      | Development docs explain package-local and PTY limits; `just coverage-integration` supplies a broader diagnostic.                                                                        |
| [Production `prototype` package and large `app.go`](#p3--the-production-app-still-presents-itself-as-a-disposable-prototype) | Do not do | The disposable comment is gone and responsibilities were split into files. A further rename, split, or options constructor has no demonstrated benefit.                                  |
| [Quality inputs and pinned tools](#p3--some-quality-inputs-are-outside-the-documented-quality-gate)                          | Done      | Container images are pinned by digest; both review documents are in Just's Markdown format checks and all review documents are linted.                                                   |
| [Prompt focus and buffer ownership](#p1--a-command-prompt-can-desynchronize-an-opened-pane)                                  | Done      | Both fixed panes are synchronized after Micro buffer replacements; tests cover focus changes and deferred prompts.                                                                       |
| [Resize during file prompt](#p1--resizing-during-micros-openfile-prompt-loses-the-opened-buffer)                             | Done      | Micro pane objects survive resize; tests cover OpenFile, command-edit open, and deferred replacement.                                                                                    |
| [First-draw alignment assertions](#p2--expanded-file-pair-tests-check-presence-more-than-diff-correctness)                   | Done      | File-backed cases assert spacer sides, changed pairs and spans, and changed glyph styles before and after scrolling.                                                                     |
| [README key and save instructions](#p2--readme-describes-obsolete-keys-and-file-workflows)                                   | Done      | README describes the actual Micro movement keys, mdiff diff keys, shared scrolling, and named-path saves.                                                                                |
| [Prompt rendering and wide-character cursor](#p2--prompt-rendering-duplicates-micro-and-misplaces-wide-character-cursors)    | Done      | The footer uses Micro's `InfoWindow.DisplayPromptRow`; a focused test checks visual columns and clipping.                                                                                |
| [Duplicate command-name whitelist](#further-simplification-opportunities)                                                    | Done      | `safeCommand` queries Micro's command registry. The remaining command exclusions are assessed in the separate compatibility review.                                                      |
| [Duplicate soft-wrap source-rune mapping](#further-simplification-opportunities)                                             | To do     | `sourceRuneLocations` still mirrors Micro's wrap-position logic. A shared API must keep linear per-line work.                                                                            |
| [Prompt at terminal height two](#p3--a-two-row-terminal-has-no-visible-prompt)                                               | Done      | The prompt temporarily uses the second row; a test checks visibility and restoration.                                                                                                    |
| [Footer menu visual width](#low-priority-simplification-and-coverage)                                                        | Done      | Menu, status, and help painting use visual-cell widths; a test covers wide menu text.                                                                                                    |
| [Coarse changes on terminal-default themes](#p2--a-terminal-default-theme-can-hide-coarse-changed-rows)                      | Done      | The divider marks changed and spacer visual rows using Micro's diff foreground colors; a test covers a coarse pair without character spans.                                              |
| [Single post-TASK-010 change and version bump](#third-re-review-of-the-current-checkout)                                     | Done      | The working copy has one Jujutsu change after TASK-010 and one `v0.0.2` application/release-notes section.                                                                               |

The normal Go suite, race suite, application-package vet, and run-argument check
passed in the [latest re-review](#eleventh-re-review-of-the-current-checkout).
The CI matrix includes macOS, but this local review did not independently run
macOS, every terminal, or every plugin. Those are verification limits, not
resolved-by-test claims.

## Scope and conclusion

I reviewed the command, the mdiff-owned comparison, viewport, coordinator,
rendering, editing, file, configuration, and plugin paths; the copied Micro test
suite; the development recipes; and CI. I ran the full Go tests, race detector,
vet, and a coverage run. This is a review of the current checkout, not a claim
that every terminal, plugin, or platform behavior was exercised locally.

The project has substantial automated testing and a stronger application-level
CI matrix than the pinned Micro Makefile's `test` target. Its copied Micro tests
also run under `go test ./...`. The present suite nevertheless misses a
high-impact startup defect, so I would not yet call the *behavioral assurance*
equivalent to the intended Micro integration. Fix the startup case and add a
multiline file-backed regression before treating the test standard as met.

## Findings, in priority order

### P1 — File-backed startup never builds the initial diff

[`NewWithFiles`](../internal/prototype/app.go) loads the two buffers, then sets
`app.pairs` to the alignment of two empty strings at line 217. It snapshots the
actual files at line 220 and calls `AcceptCurrent` at line 221, which clears the
coordinator's dirty state. `resize` then expands the one placeholder pair. No
initial comparison is queued, so scrolling, diff navigation, and rendering
cannot reach later file lines until a text edit causes a new comparison. The
one-line [startup tests](../internal/prototype/file_workflow_test.go) and
[file-argument PTY test](../internal/prototype/pty_test.go) check file identity
and first-line content but do not assert an initial multiline layout.

**Reproduction:** a temporary, removed test opened a four-line left file and a
five-line right file with an inserted line. Immediately after `NewWithFiles`, it
logged `left=4 right=5; aligned pairs=1; visual rows=1; stale=false`. This is a
confirmed defect, not a timing hypothesis.

**Fix:** build the initial pairs from the actual snapshots before `resize`, or
start an asynchronous comparison while exposing a complete coarse layout until
it finishes. Keep `layoutVersions` and coordinator state consistent with the
chosen path. Add a test that opens different multiline files and checks every
source line has a row, the insertion has a spacer, scrolling reaches the last
line, and the first draw is aligned before any edit. Include an unchanged
multiline pair and one-file startup.

### P2 — Line-level diff correctness has limited input breadth

[`internal/compare/alignment_test.go`](../internal/compare/alignment_test.go)
checks insertion, replacement/deletion, identity, a high-token-count insertion,
and whole-line spans with a few hand-selected left/right inputs. Its
`TestDiffLineChangedSpans` table has eight character-level cases, and its fuzz
target checks character-span reconstruction. The deterministic 500-case
[`viewport` property test](../internal/viewport/layout_test.go) generates short,
repeated left/right line sequences, but asserts logical/visual coordinate
mapping rather than the correctness of each diff kind and changed range.
Rendering tests cover selected Unicode, tab, wrap, and style cases. These are
valuable tests, but they do not yet constitute extensive pairwise validation of
the full line diff and rendered outcome. The startup defect above demonstrates
the difference between testing `Align` in isolation and testing the displayed
diff for supplied files.

**Fix:** add a compact corpus of explicit expected alignments for multiple
separated hunks, repeated and reordered blocks, insertions/deletions on both
sides, empty and final lines, Unicode, and asymmetric line counts. Add a
deterministic generated test for small left/right inputs that checks both source
projections are complete and ordered, equal rows truly match, changed rows refer
to valid unequal lines, and character spans reconstruct both lines. Compare
small inputs against an independent reference or edit-distance oracle where a
unique expected alignment exists. Then exercise representative pairs through
first draw and scrolling, not only through `compare.Align`.

### P2 — Documented `just run` invocation fails; flags are swallowed

The [recipe](../justfile) at line 116 inserts a literal `--` before all user
arguments. Just itself also preserves the `--` in the documented
[`just run -- left.txt right.txt`](../README.md) example at line 29. The dry run
expands to `go run -buildvcs=false ./cmd/mdiff -- -- left.txt right.txt`, and
running that command exits with `mdiff accepts at most two file paths`. Even
`just run --config-dir /tmp` expands with the recipe's first `--`, so the CLI
treats the flag as a path. The CI smoke test calls `go run` directly and does
not exercise this recipe. The unquoted `{{ args }}` expansion also splits a
single file path containing spaces (`just --dry-run run 'left with space.txt'`
shows separate shell words).

**Fix:** remove the unconditional separator, forward each argument safely, and
document the working Just argument form. Check two-file launch, a real CLI flag,
and a path containing spaces, ideally with a lightweight command-line smoke
check that does not require an interactive terminal.

### P2 — Race and fuzz checks exist but are absent from the CI gate

[`just test-race`](../justfile) runs `go test -race ./...`, and the code has a
[`DiffLine` fuzz target](../internal/compare/alignment_test.go), but
[CI](../.github/workflows/ci.yml) runs neither a race pass nor a bounded fuzz
campaign. Ordinary `go test` executes fuzz seeds, not generated cases. This is
especially relevant to the snapshot worker, Micro's asynchronous syntax path,
and global plugin state. The local race run passed, so this is a gate gap, not a
reported race.

**Fix:** add a Linux race job for at least `internal/prototype` and
`internal/compare` (prefer `./...` if runtime permits). Run a bounded, seeded
fuzz campaign periodically or before release. Keep the normal matrix fast.

### P3 — Coverage reports need an explicit interpretation

The local `-coverprofile` run on Go 1.26.9 reported statement coverage of 90.9%
for `cmd/mdiff`, 96.6% for `internal/compare`, 95.0% for `internal/viewport`,
and 86.6% for `internal/prototype`. These are useful package-local figures, but
`internal/micro/action`, `display`, `screen`, and other copied packages show 0%
in that report because they have no tests in their own packages. The full
profile's 22.0% total is therefore not a useful measure of mdiff's integration
coverage. The `RunWithSignals` function also shows 0% because the PTY tests
launch a separate, uninstrumented binary.

**Fix:** retain the package figures as a diagnostic, and add a separate
integration coverage run using `-coverpkg` over the application and copied Micro
packages if a numerical gate is desired. Prioritize behavior cases such as
startup over a target percentage. Do not infer that the copied Micro code is
untested solely from the package-local zeroes.

### P3 — The production app still presents itself as a disposable prototype

[`internal/prototype/app.go`](../internal/prototype/app.go) begins with a
"disposable TASK-002 ... probe" package comment, while the shipped CLI imports
it and it owns the entire production event loop. The file is 1,619 lines and
mixes startup, configuration, file workflow, event dispatch, viewport anchors,
menus, and terminal rendering. This raises the cost of reviewing state changes
like the missed startup initialization.

**Simplification:** rename the package to reflect its production role when
convenient, and split `app.go` by responsibility within the same package first.
Keep one clear initialization path that creates buffers, snapshots, the initial
diff, and the first layout in order. The `New`/`NewWithDebounce`/`NewWithFiles`
and `Run` variants can later be reduced to one options-based entry point; these
are internal APIs, so no public compatibility promise is involved. Preserve the
existing behavioral tests during the move.

### P3 — Some quality inputs are outside the documented quality gate

The [Just recipes](../justfile) format and lint `README.md`, `docs`, and
`design_docs`, but omit `review_docs`, including this review. The YAML and
Markdown lint containers use mutable `:latest` tags at lines 32, 49, 62, and 66.
A future image update can change CI results without a source change.

**Simplification:** decide whether `review_docs` is an archive or maintained
documentation. If maintained, include it in formatting and linting after
normalizing the existing files. Pin the container images to versions or digests
so the gate is reproducible. These are maintenance issues, not observed product
failures.

## Testing compared with Micro

The pinned
[Micro v2.0.15 Makefile](https://github.com/micro-editor/micro/blob/v2.0.15/Makefile)
defines `test` as `go test ./internal/...` plus `go test ./cmd/...`; it also has
benchmark recipes and a generator for VS Code-derived buffer tests. This
checkout includes those generated buffer tests under
[`internal/micro/buffer`](../internal/micro/buffer) and runs all copied-package
tests through `go test ./...`. It is a selected source closure rather than a
full upstream checkout, so its test count is not directly comparable to all of
Micro. The comparison here uses the pinned Makefile and the copied source, not
an unverified claim about every upstream CI workflow.

| Area                           | Current mdiff evidence                                                                                   | Assessment                                                           |
| ------------------------------ | -------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Unit and generated Micro tests | `go test ./...` passed; copied buffer tests included                                                     | Meets the pinned Makefile's basic test command for included packages |
| mdiff behavior tests           | Alignment, viewport, editing, file workflows, plugin, quit, configuration, render, and PTY tests         | Broad, but multiline startup and pairwise diff breadth are missing   |
| Cross-platform CI              | Linux/macOS × Go 1.26/1.27 tests, PTY, vet, build, CLI smoke                                             | Stronger matrix than the Micro Makefile itself specifies             |
| Concurrency                    | `go test -race ./...` passed locally                                                                     | Good local check; add CI enforcement                                 |
| Generated/property tests       | Micro generated buffer tests, `DiffLine` fuzz seeds, and a deterministic 500-case viewport property test | Useful; generated fuzz cases are not run by normal CI                |
| Performance                    | Deterministic benchmarks and documented reference measurements                                           | Good evidence; benchmark thresholds are not CI gates                 |

The highest-value missing test is a **first-draw, multiline file-backed
startup** test. Follow it with broader line-diff input generation and a recipe
argument check, then consider race/fuzz CI. A coverage threshold alone would
have missed the startup bug.

## Verification and limits

- `go test ./...`: passed on local Go 1.26.8.
- `go test -race ./...`: passed on local Go 1.26.8.
- `go vet -composites=false` for mdiff-owned app packages: passed.
- `GOTOOLCHAIN=go1.26.9 GOCACHE=/tmp/mdiff-review-gocache go test ./... -coverprofile=/tmp/mdiff-review-coverage.out`:
  passed; package percentages above.
- `just run -- left.txt right.txt`: reproduced the argument-count failure.
- The temporary startup probe was removed; no application or test source was
  changed as part of this review.

The local Go 1.26.8 Nix toolchain lacks `covdata`, so its coverage attempt
failed; the successful coverage run used the cached Go 1.26.9 toolchain and a
writable cache under `/tmp`. I did not run Docker-backed formatting/linting,
dependency audits, the benchmark suite, or a macOS terminal session in this
review. Existing changes in `design_docs/agent_workflow.md` and
`docs/development.md` were left untouched.

## Re-review of the current checkout

This pass inspected the subsequent project and behavior compatibility changes,
including comparison tests, the startup path, the event dispatcher, Micro
adapters, file workflow, CI, and development recipes. The original findings
above are retained as a record; the status below supersedes their conclusions.

### P1 — A command prompt can desynchronize an opened pane

[`openCommandBar`](../internal/prototype/app.go) captures the Micro pane when
the prompt opens. Mouse events can still change focus while the prompt is
active. After a command changes the captured pane's buffer,
[`runMicroCommand`](../internal/prototype/app_compat.go) and
[`handlePromptEvent`](../internal/prototype/app_files.go) call
`syncFocusedBuffer`, which synchronizes the pane that is focused *now*. The
opened pane can therefore keep a new `h.Buf` while mdiff's corresponding
`p.buf`, diff snapshots, file identity, and close path still reference the old
buffer.

**Reproduction:** a temporary test opened the command bar in the left pane,
clicked the right pane, and entered `open <path>`. The test failed with the left
Micro pane showing the new path while mdiff's left pane still owned an unnamed
buffer (`focus=1`). The probe was removed after verification.

**Fix:** synchronize the pane associated with the command's `*BufPane`,
including commands completed by a later prompt callback. Keep prompt ownership
explicit, or prevent focus changes while a prompt is active and test that
invariant. Add a regression covering `open` and another buffer-replacing command
with a focus change between prompt creation and completion.

### Status of the original findings

| Original finding               | Current evidence                                                                                                                                                                                                                                     | Status                                          |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| File-backed initial diff       | `NewWithFiles` aligns actual snapshots before the first resize; `TestFileStartupBuildsCompleteFirstDraw` checks insertion, unchanged files, and one-file startup, including last-line reachability.                                                  | Fixed                                           |
| Diff input breadth             | Eight explicit alignment pairs, 500 deterministic generated pairs with projection and span checks, 100 unique-sequence cases checked against an independent LCS count, and the existing character-span fuzz target now supplement the earlier tests. | Substantially improved; see testing limit below |
| `just run` arguments           | The recipe forwards `"$@"`; `scripts/check-run-args.sh` covers two files, a CLI flag, and a path with spaces; CI runs it.                                                                                                                            | Fixed                                           |
| Race and fuzz CI               | CI now has a Linux race job and a bounded fuzz job for workflow dispatch and the weekly schedule.                                                                                                                                                    | Fixed                                           |
| Coverage interpretation        | `just coverage-integration` measures the in-process app with the copied Micro packages; development docs explain package-local and PTY coverage limits.                                                                                              | Addressed without a numeric gate                |
| Production `prototype` package | The stale disposable comment is gone, and file, rendering, and compatibility code has been split out. `app.go` remains about 1,200 lines and the production package is still named `prototype`.                                                      | Partially simplified; low-priority maintenance  |
| Quality inputs                 | Container images are pinned by digest; development docs explicitly treat `review_docs` as historical records outside the Markdown gate.                                                                                                              | Resolved by policy                              |

### Remaining testing limit

The new alignment corpus and generated checks provide meaningful variation in
left and right texts, and the independent LCS count checks more than internal
consistency. The application-level first-draw test still uses three file pairs;
it does not vary repeated blocks, reordered blocks, Unicode changes, and
multiple separated hunks through rendering and scrolling. A compact table-driven
integration corpus would reduce the chance of another comparison-to-display
wiring error. This is narrower than the original missing diff-test finding and
does not negate the improved core comparison coverage.

The current test gate is at least as broad as the pinned Micro Makefile's basic
test command and adds a cross-platform matrix, race job, and scheduled fuzzing.
The reproduced prompt-focus defect shows that passing those gates is not yet
evidence of complete mdiff workflow coverage.

### Verification in this pass

- `GOCACHE=/tmp/mdiff-review-gocache go test ./...`: passed on Go 1.26.8.
- `GOCACHE=/tmp/mdiff-review-gocache go test -race ./...`: passed.
- `GOCACHE=/tmp/mdiff-review-gocache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`:
  passed.
- `bash scripts/check-run-args.sh`: passed.
- A five-second `DiffLine` fuzz run completed about 48,000 executions without a
  failure.
- The temporary prompt-focus probe reproduced the P1 finding and was removed.

I did not run the containerized lint/audit tools or macOS CI locally. The
remaining Micro behavior findings and their verification are kept in the
separate compatibility review.

### Follow-up resolution

The command and prompt adapters now synchronize every fixed pane whose Micro
buffer changed, including replacements completed by a later save-choice
callback. `TestPromptOwnedBufferSyncSurvivesFocusChange` covers command-bar
`open`, its deferred dirty-buffer prompt, and Micro's OpenFile prompt after
mouse focus moves to the other pane. The first-draw integration table now also
renders and scrolls repeated blocks, reordered blocks, Unicode changes, and
separated hunks. The documented low-priority package naming and file-size
maintenance remains open.

## Second re-review of the current checkout

The previous prompt-focus defect is fixed for focus changes: the new
`syncChangedBuffers` checks both fixed panes, and
[`TestPromptOwnedBufferSyncSurvivesFocusChange`](../internal/prototype/behavior_compat_test.go)
covers immediate and deferred file replacement. The expanded
[`first-draw table`](../internal/prototype/file_workflow_test.go) now exercises
seven different file pairs. The raw/mouse Quit and help changes are assessed in
the separate
[Micro behavior review](micro_behavior_compatibility_review_2026-10-13.md).

### P1 — Resizing during Micro's OpenFile prompt loses the opened buffer

[`resize`](../internal/prototype/app.go) creates new `BufPane` objects for both
panes. Micro's [`OpenFile`](../internal/micro/action/actions.go) prompt retains
the old `BufPane` in its callback. If the terminal resizes before the user
enters a path, the callback opens the file on that detached pane. The current
pane and mdiff's owned buffer remain unchanged, so the file is not displayed and
the new buffer is outside mdiff's close path.

**Reproduction:** a temporary test opened `Ctrl+O`, sent `EventResize`, entered
a valid file path, and pressed Enter. The prompt remained active through resize
and completed. The old pane's `Buf` had the opened path, while both the current
Micro pane and mdiff still had an unnamed buffer. The test failed as expected
and was removed.

**Fix:** preserve each Micro pane object across resize and update its geometry,
or make callbacks resolve the live pane by stable identity before replacing a
buffer. Test `OpenFile`, `command-edit:open`, and deferred dirty-buffer
replacement with a resize between opening and completing each prompt. Reusing
pane objects would also simplify state ownership and avoid stale callback
references elsewhere.

### P2 — Expanded file-pair tests check presence more than diff correctness

The four new first-draw fixtures cover repeated, reordered, Unicode, and
separated changes. For those cases, the test asserts that source fragments
appear somewhere after scanning scroll positions; it does not assert the
expected aligned rows, pair kinds, spacer positions, or changed spans. A
renderer that displayed both files independently could satisfy these fragment
checks. The separate
[comparison corpus](../internal/compare/alignment_properties_test.go) does check
exact alignments, so the remaining gap is specifically comparison-to-render
integration for varied inputs.

**Fix:** for a few of the new file pairs, assert the expected row map and screen
position of at least one inserted, moved, or changed line on both sides. Check a
representative diff marker or character span in the first draw and after
scrolling. Keep the broad projection properties in the comparison package.

### Verification

`go test ./...` and `go test -race ./...` passed with
`GOCACHE=/tmp/mdiff-review-gocache` on Go 1.26.8. Vet passed for the mdiff
application packages. The temporary resize probe was removed after reproducing
the P1 issue; no implementation or test source was changed by this review.

### Second follow-up resolution

The two Micro `BufPane` objects now survive resize, so in-flight OpenFile,
`command-edit:open`, and deferred dirty-buffer callbacks still target the pane
owned by mdiff. `TestFilePromptsKeepPaneOwnershipAcrossResize` covers all three
paths and checks the rebuilt diff snapshot. The first-draw fixtures now assert
both sides of a reordered spacer, changed pairs and rune spans, and changed
glyph styling on the initial draw and after scrolling. The production package
name and large `app.go` remain low-priority maintenance items.

## Third re-review of the current checkout

The working copy remains one Jujutsu change directly after TASK-010. Its parent
has `v0.0.1`, while the current application version is `v0.0.2`; the release
notes have one `v0.0.2` section. This matches the requested single change and
single version bump for post-TASK-010 work. Igor was still editing this change
at review time, so this is a snapshot of the current working copy.

The earlier resize and first-draw test gaps are resolved. Resize retains the
Micro panes used by pending file prompts, and the regression test checks Open,
command-edit Open, and dirty replacement. Startup rendering tests now assert
specific aligned rows, spacer positions, changed pairs and spans, and glyph
styles before and after scrolling for several file pairs.

### P2 — README describes obsolete keys and file workflows

The [Run section](../README.md) still says `Ctrl+Up`/`Ctrl+Down` scroll both
panes, `Alt+Up`/`Alt+Down` navigate differences, and no Save As command exists.
The [bundled Micro defaults](../internal/micro/action/defaults_other.go) bind
those arrow combinations to cursor or line movement, and the later README
configuration section correctly names `Alt-[`/`Alt-]` as the default diff keys.
The [action router](../internal/prototype/app_compat.go) also supports `SaveAs`
when bound; Micro's `save <path>` command provides another named-path save
workflow. These contradictory instructions could cause users to move or edit
text while expecting diff navigation. Update the Run section to match the
current bindings and supported save actions.

### Verification

`go test ./...`, `go test -race ./...`, application-package vet, and
`scripts/check-run-args.sh` passed on this working copy. No implementation
source was changed by this review. Remaining package naming and large-file
simplification opportunities remain low priority.

### Third follow-up resolution

The README Run section now names the actual default `Alt-[`/`Alt-]` diff keys,
explains Micro's `Ctrl+Up`/`Ctrl+Down` and `Alt+Up`/`Alt+Down` movement, and
describes the shared mouse-wheel scroll and bindable scroll actions. It also
documents `save <path>` and the bindable `SaveAs` action. The consolidated jj
change and its single `v0.0.2` bump remain intact.

## Fourth re-review of the current checkout

The latest fixes resolve the third pass's README finding. The Run section now
describes the actual Micro movement bindings, mdiff's diff keys and shared
scrolling, and both named-path save routes. The working copy remains one Jujutsu
change directly after TASK-010, with a single application version bump from
`v0.0.1` to `v0.0.2` and one matching release-notes section.

I found no new project correctness or test-standard issue in this pass. The
comparison suite now includes explicit line-pair expectations, generated
projection checks, and an independent LCS check for unique-line inputs. The
file-backed startup tests assert aligned rows and changed rendering on first
draw and after scrolling; the resize tests cover prompts that outlive a resize.
CI includes normal tests, a race job, and a scheduled bounded fuzz campaign.
This is substantial automated coverage alongside the copied Micro tests, though
the local run does not establish results for other operating systems or every
terminal and plugin combination.

`go test ./...`, `go test -race ./...`, application-package vet, and
`scripts/check-run-args.sh` passed. A temporary help-search probe was removed;
no implementation source was changed by this review. The production `prototype`
package still has a 1,206-line `app.go`, and `review_docs` is still outside the
Just formatting gate. Those remain maintenance simplification opportunities
rather than new failures. The remaining help-search behavior difference is
recorded in the separate
[Micro compatibility review](micro_behavior_compatibility_review_2026-10-13.md#fourth-independent-re-review).

### Fourth follow-up resolution

This pass identified no new project correctness issue. The help-search behavior
finding is addressed in the Micro compatibility follow-up below, without a new
Jujutsu change or version bump. The `prototype` package size and review-doc
formatting policy remain the recorded low-priority maintenance items.

## Fifth re-review and Micro reuse audit

Igor's latest help-search change resolves the fourth pass's regex and case
finding. During this review I extracted the regex compilation rule into Micro's
buffer search package, so Micro's `FindNext` and the mdiff help overlay now use
one compiler. I also changed the renderer to retain Micro's focused line-number
style and to use lighter current-line diff colors; those changes are documented
in the separate compatibility review. The user asked that these source changes
be kept, and the remaining items below are review findings only. Full Go tests,
race tests, application-package vet, and the run-argument check pass. The
working copy is still one post-TASK-010 Jujutsu change with one `v0.0.2` bump.

### P2 — Prompt rendering duplicates Micro and misplaces wide-character cursors

mdiff's [promptDisplay and renderLine](../internal/prototype/app_render.go)
count runes as terminal columns. Micro's
[InfoWindow](../internal/micro/display/infowindow.go) already handles visual
width, tabs, combining characters, prompt styles, and cursor placement. A
temporary, removed probe entered `猫` in mdiff's command prompt: the terminal
cursor appeared at column 3 where the next input cell is column 4. This affects
prompts for non-ASCII commands, searches, and paths; longer mixed-width input
can also clip incorrectly. Adapt Micro's prompt-row rendering to mdiff's footer,
or extract its reusable input renderer, while keeping mdiff's two-pane menu
placement. Add a focused wide-character prompt test.

### Further simplification opportunities

- [sourceRuneLocations](../internal/prototype/render.go) explicitly mirrors
  Micro's wrap and visual-position calculation in
  [BufWindow](../internal/micro/display/softwrap.go). A shared one-pass visual
  location API would remove a large compatibility-sensitive copy. Preserve its
  linear cost for long lines; calling Micro's current single-location method
  once per rune would be quadratic.
- [safeCommand](../internal/prototype/app.go) repeats Micro's built-in command
  names as a whitelist. Expose a registered-command query from
  [Micro's registry](../internal/micro/action/command.go), then keep a narrow
  mdiff denylist for commands that add views or take over the terminal. Ordinary
  Micro commands can then continue through their existing handlers without
  keeping two lists in sync.

Micro's buffer editing, file actions, command handlers, configuration, and
BufWindow painting are already delegated to Micro where the two-pane model
allows it. The shared aligned viewport, diff composition, and sequential
two-pane quit/save workflow still require mdiff-owned coordination.

### Fifth follow-up resolution

mdiff now uses Micro's `InfoWindow.DisplayPromptRow` for its footer prompt. That
renderer measures visual columns for wide characters, combining marks, and tabs,
clips long input at character boundaries, and returns the cursor column for
mdiff's final frame. `TestPromptCursorUsesVisualColumnsAndClipsMixedWidthInput`
checks wide input and a long mixed-width prompt. The two-pane menu stays in
mdiff's footer rows. The general status/help line painter now also advances by
visual columns and preserves combining marks; `TestStatusLineUsesVisualColumns`
checks wide, combined, and tabbed text.

`safeCommand` now asks Micro's command registry whether a command exists, then
rejects only commands that require extra views or unsupported terminal handoff.
The source-rune mapping still uses a linear one-pass implementation; the
proposed shared wrap-position API remains a maintenance improvement for Micro's
renderer and mdiff's diff annotation.

## Sixth re-review of the current checkout

Igor's prompt renderer, command-registry lookup, and fake-cursor changes resolve
the fifth pass's actionable findings. The full Go suite, race suite,
application-package vet, and `scripts/check-run-args.sh` pass on this working
copy. It remains one Jujutsu change directly after TASK-010, with the single
`v0.0.2` version bump. This pass changed review documents only.

### P3 — A two-row terminal has no visible prompt

[`resize`](../internal/prototype/app.go) accepts a minimum height of two rows,
but reserves a footer row only when the height reaches three. The
[`draw` method](../internal/prototype/app_render.go) therefore never paints an
active command, search, open-file, or help-search prompt at height two.
`cursorPosition` still reports the prompt cursor as visible at `statusRow=-1`.
This is a source-level edge case that the current prompt tests do not cover.
Reserve one row while a prompt is active, or define and test another visible
prompt placement for the two-row layout.

### Low-priority simplification and coverage

The footer's [`renderMenuLine`](../internal/prototype/app_render.go) still
measures and clips by rune count, while the adjacent status line and Micro's
prompt renderer use terminal-cell widths. Current built-in menu labels and
supported single-key bindings are ASCII, so this is a future-proofing concern
rather than a demonstrated default-path defect. If menu text grows to include
wide characters, reuse a visual-cell painter instead of maintaining a second
text-width rule. The previously recorded `sourceRuneLocations` and Micro
soft-wrap mapping duplication is still the clearer simplification opportunity.

The separate
[Micro compatibility review](micro_behavior_compatibility_review_2026-10-13.md#sixth-independent-re-review)
records the help-overlay display differences found in this pass.

### Sixth follow-up resolution

At a two-row terminal height, opening a prompt now temporarily reserves the
second row for Micro's InfoBar and leaves one editor row visible. Closing the
prompt restores the two editor rows.
`TestTwoRowTerminalShowsPromptAndRestoresEditorRows` checks prompt text, cursor
position, and restoration.

The footer menu now clips and aligns by terminal-cell width using the same
character painter as the status and help rows.
`TestMenuLineAlignsWideTextByVisualColumns` checks wide text on both sides of
the menu. The earlier one-pass `sourceRuneLocations` implementation remains
linear; moving that mapping into a shared Micro API is still a maintenance
improvement, rather than a behavior failure found in this pass.

The two current review documents are now formatted and included in Just's
Markdown format checks. All review documents are included in Markdown linting.

## Seventh re-review of the current checkout

Igor's sixth follow-up fixes the two-row prompt and footer text-width findings,
with focused regressions. The prompt uses the second row only while active; the
footer's menu, status, and help text now use visual-cell widths. The full Go
suite, race suite, application-package vet, and run-argument check pass. The
working copy is still one Jujutsu change after TASK-010 with the single `v0.0.2`
bump. This review changed documents only.

I found no new general correctness or test-standard defect in this pass. The
alignment suite still checks varied left and right inputs through explicit pair
expectations, generated projection properties, an independent LCS oracle for
unique lines, and character-span reconstruction. The remaining
`sourceRuneLocations` copy of Micro's soft-wrap position logic is a maintenance
simplification opportunity, provided a shared API can preserve linear work per
line. The separate
[Micro compatibility review](micro_behavior_compatibility_review_2026-10-13.md#seventh-independent-re-review)
records one narrow help-search mapping issue.

### Seventh follow-up resolution

The help-search mapping issue is resolved: a source tab now records the first
and last visual rows occupied by its expanded spaces, so a match highlights both
rows when the tab wraps. The approved dark diff palette and optional Micro
colorscheme row-background groups are implemented and documented in README; the
detailed palette resolution is in the Micro compatibility review. The single
post-TASK-010 Jujutsu change and `v0.0.2` bump remain intact. The one-pass
source-rune mapping remains a maintenance refactor, with no behavior failure
identified in this re-review.

## Eighth re-review of the current checkout

The latest help-search tab mapping, approved dark diff palette, and optional
Micro colorscheme groups have focused tests. `go test ./...`,
`go test -race ./...`, application-package vet, and `scripts/check-run-args.sh`
pass. The working copy remains one Jujutsu change after TASK-010 with a single
`v0.0.2` bump. This pass changed review documents only.

I found no new alignment or file-workflow correctness issue. The new color tests
cover approved dark fallbacks and a custom theme override, but do not cover an
existing bundled light Micro theme without `mdiff.diff.*` entries. That missing
case exposes a readability regression described in the separate
[Micro compatibility review](micro_behavior_compatibility_review_2026-10-13.md#ninth-independent-re-review).
The user clarified that equal rows must retain Micro's ordinary colors, which
also makes the new `mdiff.diff.equal` group unnecessary. That requirement is
recorded in the compatibility review. The shared soft-wrap location API remains
a maintenance simplification opportunity, provided it retains the current linear
mapping cost.

### Eighth follow-up resolution

Equal rows now use Micro's normal foreground and background on both sides;
focused equal rows retain Micro's `cursor-line` background. The
`mdiff.diff.equal` group and dark equal-row fallback were removed. Existing
light themes without mdiff groups receive pale changed and spacer backgrounds
blended from the theme's default background. The approved dark palette and
explicit Micro colorscheme overrides remain. A renderer test loads bundled
`dukelight-tc` unchanged and checks equal, changed, and spacer rows as well as
Micro's character and line-number foregrounds. The work remains in the single
post-TASK-010 Jujutsu change with one `v0.0.2` bump.

## Ninth re-review of the current checkout

Igor's equal-row change restores Micro's normal colors on both sides, and the
new light-theme fallback passes its focused renderer test. The full Go suite,
race suite, application-package vet, and run-argument check pass. The working
copy is still one Jujutsu change directly after TASK-010 with a single `v0.0.2`
bump. This review changed documents only.

I found no new line-alignment or file-workflow issue. The theme tests now cover
an explicit light Micro background, but not a bundled theme that leaves its
background to the terminal. That case is the remaining compatibility finding in
the separate
[Micro behavior review](micro_behavior_compatibility_review_2026-10-13.md#tenth-independent-re-review).
The shared soft-wrap location API is still a low-priority simplification
opportunity if it can keep the current linear mapping cost.

### Ninth follow-up resolution

When Micro's default background has no RGB value, diff rows now retain the
terminal background instead of assuming a dark terminal. Explicit `mdiff.diff.*`
groups still override it; themes with known dark or light backgrounds keep their
existing fallbacks. A renderer test loads bundled `simple`, verifies equal,
changed, and spacer backgrounds and Micro's text styles, and checks explicit
overrides. The detailed behavior is recorded in the Micro compatibility review.
This remains the same post-TASK-010 Jujutsu change and the single `v0.0.2` bump.

## Tenth re-review of the current checkout

Igor's terminal-default theme fallback resolves the ninth pass's contrast
finding: an unmodified `simple` Micro theme retains the terminal background,
while explicit mdiff theme groups still override it. The full Go suite, race
suite, application-package vet, and run-argument check pass. The working copy
remains one Jujutsu change after TASK-010 with the single `v0.0.2` bump. This
review changed documents only.

### P2 — A terminal-default theme can hide coarse changed rows

[`renderPane`](../internal/prototype/render.go) uses `diffStyle` as its only
whole-row indication of `Changed` or `Spacer`. With an unknown terminal
background, the new fallback makes those row backgrounds identical to equal
rows. Normal comparisons still color changed characters, but
[`coarseAlignment`](../internal/compare/alignment.go) produces changed pairs
without `Changes` spans when the comparison budget expires. Such rows have
neither character highlighting nor a whole-row marker under bundled `simple` and
similar themes. The status warning reports a coarse region but does not identify
its rows; blank spacer rows likewise lose their red/green distinction. The new
`simple` renderer test asserts the shared terminal background and foreground
retention, but not that non-equal rows remain identifiable.

Keep the terminal-default background and give every non-equal row a visible
theme-compatible cue, such as a narrow gutter marker using Micro's existing
`diff-modified`, `diff-deleted`, and `diff-added` styles. Preserve Micro's
normal colors on equal rows. Add a focused terminal-default rendering test for
coarse changed pairs with no character spans and for spacer rows, asserting the
cue remains visible. The one-pass soft-wrap location mapping remains a separate
low-priority simplification opportunity.

### Tenth follow-up resolution

Terminal-default themes now mark every non-equal visual row in the existing
center divider, without changing either pane's terminal background. `!` marks
changed pairs, `<` marks left-only rows, and `>` marks right-only rows. The
markers use Micro's `diff-modified`, `diff-deleted`, and `diff-added` foreground
styles. Equal rows retain the ordinary divider and Micro colors. A focused
renderer test supplies a coarse changed pair with no character spans plus both
spacer directions, then checks the markers and unchanged terminal backgrounds.
The single post-TASK-010 Jujutsu change and `v0.0.2` bump remain intact.

## Eleventh re-review of the current checkout

The center-divider markers resolve the coarse-diff visibility finding from the
tenth pass. `diffDivider` checks the expanded visual row kind, so a changed pair
without character spans remains marked even when its text uses the terminal
background; wrapped rows also retain the marker. The marker direction matches
the spacer side, and its foreground comes from Micro's existing diff color
groups. Equal rows keep the ordinary divider and Micro's normal colors. The
focused renderer test covers a coarse changed pair and both spacer directions. I
found no new correctness or testing finding in this pass.

`go test ./...`, `go test -race ./...`, application-package vet, and
`scripts/check-run-args.sh` pass. The working copy remains one Jujutsu change
after TASK-010 with a single `v0.0.2` bump. The one-pass `sourceRuneLocations`
copy of Micro's soft-wrap mapping remains a low-priority maintenance
simplification opportunity; this pass changed review documents only.

## Pre-merge configuration follow-up

Startup now creates a missing selected default Micro config directory before
loading settings and plugins. Explicit `--config-dir` paths continue to require
an existing directory. Focused tests cover the Micro, XDG, and home fallback
paths, a creation error, and command-history persistence after a first run. The
existing config-file test still checks that startup does not replace user files.
This stays in the single post-TASK-010 Jujutsu change with one `v0.0.2` bump.
