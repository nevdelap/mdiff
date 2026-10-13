# TASK-009 Review

**Task:** Meet large-input performance and resource targets  
**Reviewer:** Rufus  
**Review pass:** 2  
**Decision:** APPROVED

## Findings

### 1. Medium — replacing a pane buffer reuses stale line-height identities

**Status:** Resolved in pass 2

`syncPaneBuffer` now resets the affected side's line identities and removes its
old cache entries when installing a different buffer. The regression test
caches a multi-row line, replaces the pane buffer with a one-row line at the
same index and geometry, then verifies the new height and aligned row count.

Evidence: [`syncPaneBuffer`](/home/nevd/mdiff/internal/prototype/app.go:910)
calls `lineHeightCache.reset` on replacement;
[`lineHeightCache.reset`](/home/nevd/mdiff/internal/prototype/lineheight.go:57)
removes entries for that side and assigns fresh IDs. Coverage is in
[`TestSyncPaneBufferResetsLineHeightCacheOnReplacement`](/home/nevd/mdiff/internal/prototype/app_test.go:178).

## Review

The deterministic fixtures and report cover the specified inputs and document
the constrained reference environment. The local benchmark rerun met the paste,
comparison-budget, and RSS targets. Comparison work has a finite budget and
coarse fallback, and the coordinator bounds pending work. Pane-buffer
replacement now invalidates cached geometry. TASK-009 meets its acceptance
criteria.

## Verification

- `just format`: passed.
- `PATH=/home/nevd/go/bin:$PATH just lint`: passed, including Go lint (0
  issues), Markdownlint, and yamllint.
- `just test`: passed, including the new pane-buffer replacement regression.
- `just test-race`: passed.
- `just test-pty`: passed.
- `just build`: passed.
- `GOMAXPROCS=4 just bench`: passed. Repeated benchmark fallbacks completed in
  about 1.53–1.56 s; 1 MiB paste-to-visible took about 219–226 ms; RSS-gate
  peak readings were 46,272,512–54,001,664 bytes.
- Docker/runtime checks ran with escalation.
