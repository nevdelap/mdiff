# TASK-014 implementation review — 2026-10-16

**Pass:** 1 and 2\
**Current decision:** Approved\
**Change:** `vskoxtsq`, Share Micro soft-wrap source mapping

The shared scanner removes mdiff's duplicate width and wrap calculation. Its
batch map uses Micro decoded-character offsets alongside Go rune offsets, and
the renderer translates selections and wrap-height lookups. The version and
release note each advance once to `v0.0.5`. Pass 1 findings and their fixes
remain below as review history; both are resolved in pass 2.

## Findings

### 1. Medium — Negative Micro locations now mean end of line

[`getVLocFromLoc`](../internal/micro/display/softwrap.go) passes `loc.X`
straight to `scanSourceLine`. The scanner reserves `-1` as its private
end-of-line sentinel
([`sourcepositions.go`](../internal/micro/display/sourcepositions.go)). Before
this change, both Micro paths returned the start of the line for `loc.X <= 0`:
the soft-wrap path checked that condition directly, and the unwrapped path used
`util.StringWidth`, which does the same. Now
`VLocFromLoc(buffer.Loc{X: -1, Y: 0})` returns the line end for nonempty lines.
Micro also uses `X: -1` as an internal buffer-location sentinel. This is an
unrequested change to Micro position behavior, even if ordinary cursor locations
are nonnegative.

**Required:** Preserve Micro's `X <= 0` behavior at the public
`VLocFromLoc`/`SLocFromLoc` path while keeping the private line-end operation.
Add a regression for `X: -1` with soft wrap on and off.

### 2. Medium — Unwrapped annotation loses its visible-width bound

The prior unwrapped renderer stopped annotating once it reached the visible
right edge. Now [`renderPane`](../internal/prototype/render.go) asks
`RuneBoundaries` for the entire source line whenever a visible row has a changed
span or the pane has a selection; `annotateSourceRunes` then walks every rune on
an unwrapped line. The map allocates two growing slices for all rune boundaries
([`sourcepositions.go`](../internal/micro/display/sourcepositions.go)). Thus a
narrow viewport over a very long unwrapped changed line pays for the whole
source line on each redraw. The submitted benchmarks measure wrapped lines only.
Even the 24,576-byte wrapped fixture allocates 2.62 MB per batch map; this makes
the unwrapped path worth protecting before approval.

**Required:** Make unwrapped annotation stop after the visible right edge while
using Micro's source-position rules. Add an unwrapped long-line rendering
benchmark and a regression that scrolls horizontally across a changed or
selected long line, confirming that cell highlighting remains correct.

## Acceptance and verification

The one-pass batch API, character/rune translation, combining-mark tests,
wrapped-line benchmarks, and single version bump meet the corresponding TASK-014
criteria. Pass 1 decision: changes required. Pass 2 confirms both findings are
resolved.

- `GOCACHE=/tmp/mdiff-go-review-cache go test ./...` — passed.
- `GOCACHE=/tmp/mdiff-go-review-cache go test ./internal/micro/display -run '^$' -bench 'Benchmark(RuneBoundariesLongLine|CursorPositionLongLine)$' -benchmem -benchtime=100ms -count=1`
  — passed. At 512/1024/2048 groups, batch scans took 0.56/1.32/2.67 ms and
  allocated 0.56/1.48/2.62 MB; cursor queries took 0.095/0.189/0.379 ms and
  allocated 1.9/3.5/6.7 kB.
- `GOCACHE=/tmp/mdiff-go-review-cache just build` — passed.
- `GOCACHE=/tmp/mdiff-go-review-cache go vet -composites=false ./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport`
  — passed.
- `uv tool run --offline ... mdformat --check` on this review document and the
  implementation plan — passed with the repository's pinned formatter versions.
- `just lint` — could not complete in this environment. After setting a writable
  runtime directory and enabling offline `uv`, `just format-check` reached its
  Docker step but access to `/var/run/docker.sock` was denied.

The existing `BenchmarkPasteOneMiBVisible` is not an unwrapped annotation
measurement: it draws immediately after a paste, before the new diff result is
installed. It passed but does not resolve finding 2.

## Implementation response

The implementer addressed both findings in change `vskoxtsq`; pass 2 verified
the code, regressions, benchmark fixture, and task handoff.

- **Finding 1 — addressed:** `getVLocFromLoc` preserves the prior `X <= 0`
  behavior, while `LineEndVLoc` uses a separate private end-of-line request.
  `TestNegativeMicroLocationsRemainAtLineStart` covers `X: -1` with soft wrap
  enabled and disabled, through both `VLocFromLoc` and `SLocFromLoc`.
- **Finding 2 — addressed:** unwrapped panes call `VisibleRuneBoundaries`, which
  scans to the visible range while retaining only intersecting rune/character
  boundaries and stops at the right edge. Absolute rune offsets preserve diff
  spans after horizontal scrolling.
  `TestUnwrappedHorizontalScrollHighlightsChangedLongLine` verifies a changed
  cell after scrolling into a 10,001-character line. The new
  `BenchmarkRenderUnwrappedLongLineAnnotations` redraws two 1,048,577-character
  unwrapped lines around their changed midpoint.

Verification after the fixes:

- `just format`, `just lint`, `just test`, and `just build` — passed.

- Focused review regressions — passed:

  ```sh
  go test ./internal/micro/display ./internal/prototype -run 'Test(Negative|Visible|UnwrappedHorizontal|RuneBoundaries|SelectionAfter|ChangedTextSpanning|MarkOnlyChange)' -count=1
  ```

- Unwrapped render benchmark on Linux amd64, Ryzen 7 5700U, `-benchtime=500ms` —
  182.5 ms/op, 10.8 MB/op, 3312 allocs/op. This measures full redraw of both
  panes; the source-position map is bounded by visible width.

## Pass 2 re-review

| Finding                      | Status   | Evidence                                                                                                                                                                                                                                                                                                                                                                                                |
| ---------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1. Negative Micro locations  | Resolved | `getVLocFromLoc` returns the line start for `X <= 0`, while `LineEndVLoc` retains the separate end-of-line scan. `TestNegativeMicroLocationsRemainAtLineStart` covers `X: -1` with soft wrap on and off through both public location methods.                                                                                                                                                           |
| 2. Unwrapped annotation work | Resolved | `renderPane` uses `VisibleRuneBoundaries` when soft wrap is off. The scanner retains only characters intersecting the visible range and stops at its right edge; absolute rune offsets preserve change-span indexing. Tests cover visible-map offsets and changed-cell highlighting after horizontal scroll. The new benchmark redraws two 1,048,577-character unwrapped lines with a changed midpoint. |

The implementation handoff reports that formatting, lint, tests, build, focused
regressions, and the unwrapped benchmark passed after the fixes. The benchmark
records full redraw cost; the visible source map is restricted to the viewport.
The review checked the implementation and coverage against both findings; the
tests were not rerun during this pass.

**Decision:** Approved. TASK-014 meets its acceptance criteria, and both pass-1
findings are resolved.
