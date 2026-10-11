# TASK-004 Review

**Task:** Add versioned debounced diff updates\
**Reviewer:** Rufus\
**Review pass:** 2\
**Decision:** APPROVED

## Findings

### 1. Medium — a captured pending comparison can become obsolete before it starts

**Status:** Resolved in pass 2

When an active comparison is running, `DebounceElapsed` stores its snapshot in
`pending`. If another edit arrives before the active comparison finishes,
`MarkDirty` updates `latest` and restarts the timer but leaves the older pending
snapshot in place. `Complete` then starts that stale pending snapshot before it
considers the newer dirty version. This can make an unnecessary full-buffer
comparison delay the newest alignment, especially for large inputs.

Reproduction sequence: start comparison for version 1; let the debounce expire
for version 2 so it becomes pending; edit again to version 3 before version 1
finishes; then complete version 1. The coordinator starts version 2 even though
version 3 is now latest.

**Required resolution:** Invalidate or replace an older pending snapshot when a
newer version is marked dirty. On active completion, schedule only the latest
snapshot according to the debounce contract. Add a deterministic coordinator
test for the sequence above and assert the version-2 comparison is never
started.

Resolution: [`MarkDirty`](/home/nevd/mdiff/internal/prototype/coordinator.go:96)
now clears a captured pending snapshot when a newer edit arrives, and
[`Complete`](/home/nevd/mdiff/internal/prototype/coordinator.go:174) only starts
a pending snapshot if its versions still match `latest`. When the pending
snapshot was invalidated and the active result is obsolete, `Complete` asks the
app goroutine to capture the latest buffers immediately. The deterministic
[`TestNewEditInvalidatesCapturedPendingSnapshot`](/home/nevd/mdiff/internal/prototype/coordinator_test.go:215)
reproduces v1 active, v2 pending, and v3 edited before v1 completes; it asserts
that v3 starts and no v2 comparison starts.

## Review

The buffer version notifications, debounce configuration and paste thresholds,
snapshot ownership, stale-result rejection, error handling, and worker shutdown
match TASK-004's requirements. The finding is resolved and all required local
verification gates pass. macOS verification remains assigned to the existing CI
matrix.

## Verification

- `GOCACHE=/tmp/mdiff-rufus-task004-cache just test`: passed (pass 2).
- `GOCACHE=/tmp/mdiff-rufus-task004-cache just test-race`: passed (pass 2).
- `GOCACHE=/tmp/mdiff-rufus-task004-cache just build`: passed (pass 2).
- `PATH=/tmp/mdiff-gobin:$PATH GOCACHE=/tmp/mdiff-rufus-task004-cache GOLANGCI_LINT_CACHE=/tmp/mdiff-rufus-lint-cache XDG_RUNTIME_DIR=/tmp just lint`:
  passed, including format-check, vet, golangci-lint (0 issues), Markdownlint,
  and yamllint.
- macOS verification was not available locally; the implementation plan assigns
  it to the existing CI matrix.
