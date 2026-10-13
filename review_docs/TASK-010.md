# TASK-010 Review

**Task:** Complete release readiness and platform verification  
**Reviewer:** Rufus  
**Review pass:** 3  
**Decision:** APPROVED

## Findings

### 1. High — hosted verification did not cover the current candidate

**Status:** Resolved in pass 3

The handoff now records a full passing [CI run 38344783009](https://github.com/nevdelap/mdiff/actions/runs/38344783009)
and both release hashes for revision `5b042d733a51a57402fc32045f40d0f8b1857408`.
However, a later [run 38345016823](https://github.com/nevdelap/mdiff/actions/runs/38345016823)
exposed a PTY transcript race, and the current candidate
changes the PTY decoder and resize checkpoint after that run. The current
candidate subsequently passed the full workflow in [run 38345278723](https://github.com/nevdelap/mdiff/actions/runs/38345278723)
on revision `aea963bfceb099286633841ffd684b2d665b7d21`.

The run passed all eight jobs: Ubuntu 24.04 and macOS 15 test/PTy jobs on Go
1.26.x and 1.27.x, quality checks, dependency audit, and the two reproducible
release-build jobs. Each release job ran Go 1.26.9 and compared two builds with
`cmp`.

| Runner/target | SHA-256 |
| --- | --- |
| Ubuntu 24.04 / `linux/amd64` | `99ea3d8acfd711aa1cc1ad52c27e30323ca6bdc779c5f18fba1ae3d8f7b13d50` |
| macOS 15 / `darwin/arm64` | `c2c7d6dde0c305d5f9db1c84769cbab7c5bbdc83528f8a5f58ea25f0cfbc24c9` |

The platform verification finding is resolved.

## Pass 3 review

The updated resize test checks that the menu redraws on its resized row and that
the final cursor position is inside the resized right pane. The PTY screen
decoder now processes the complete transcript so checkpoints that split ANSI
sequences are handled correctly. The resize scenario passed eight consecutive
local runs, the full local check gate passed, and the exact candidate passed all
required GitHub CI jobs and release reproducibility checks. TASK-010 meets its
acceptance criteria and is approved.

### Pass 3 verification

- `PATH=/home/nevd/go/bin:$PATH just format`: passed.
- `PATH=/home/nevd/go/bin:$PATH just check`: passed, including format checks,
  Go vet, golangci-lint (0 issues), Markdown and YAML lint, tests, and build.
- `go test ./internal/prototype -run '^TestPTYPasteScrollResizeAndTerminalRestore$' -count=8`:
  passed.
- [GitHub Actions run 38345278723](https://github.com/nevdelap/mdiff/actions/runs/38345278723):
  all eight jobs passed on `aea963bfceb099286633841ffd684b2d665b7d21`. This
  includes Ubuntu 24.04 and macOS 15 PTY checks, dependency audit, and both
  runner-specific reproducible release builds with the SHA-256 values above.
- `just audit`, `just test-pty`, `go mod tidy -diff`, and local Linux release
  reproducibility passed in pass 1; module and release-build inputs are
  unchanged.

## Pass 2 history

**Decision:** CHANGES REQUIRED. The prior review was waiting for hosted CI on
the current candidate; the successful run above resolves that finding.

The resize scenario passed eight consecutive local runs, but hosted verification
for the candidate was still outstanding at that pass.

## Pass 1 history

**Decision:** CHANGES REQUIRED. The review initially reported that no hosted CI
or runner hashes existed. The implementation handoff now records successful
hosted results for an earlier revision, so that wording is superseded; the
current candidate still needs its own passing platform run.

Local release reproducibility on pass 1: two builds compared byte-identically
on `linux/amd64`, Go `go1.26.8`; SHA-256
`797bcab743008bc9853e0c1501dfb81f1fe74197cf196b7b8b1768762bf70c76`.
