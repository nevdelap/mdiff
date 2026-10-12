# TASK-008 Review

**Task:** Harden shared viewport and terminal interaction  
**Reviewer:** Rufus  
**Review pass:** 2  
**Decision:** APPROVED

## Findings

### 1. Medium — repeated-line restoration ignores the captured occurrence

**Status:** Resolved in pass 2

The anchor now tracks logical line changes from buffer text-change ranges and
refreshes its occurrence in the resulting snapshot. Restoration uses occurrence
distance after matching text and surrounding context, then numeric line
distance. A regression test inserts a repeated seven-line block above an anchor
whose three lines of context on each side match the duplicate, and verifies the
viewport remains on the original occurrence.

Evidence: [`captureAnchor`](/home/nevd/mdiff/internal/prototype/app.go:531)
captures occurrence and context; [`textChanged`](/home/nevd/mdiff/internal/prototype/app.go:267)
transforms the pending anchor through edits; [`restoreAnchor`](/home/nevd/mdiff/internal/prototype/app.go:1105)
uses occurrence distance to break contextual ties. Coverage is in
[`TestRepeatedBlockAnchorTracksOccurrenceWhenIdenticalBlockInsertedAbove`](/home/nevd/mdiff/internal/prototype/app_test.go:393).

## Review

The requested layout adjustments are present: the two pane status lines precede
the two-row Micro menu, and the InfoBar occupies the bottom row. The viewport
uses indexed shared visual coordinates, mouse spacer clicks are deterministic,
horizontal offsets remain pane-local, and repeated-block anchor restoration
now follows the selected occurrence through edits. Simulation and PTY coverage
exercise the required interactions. TASK-008 meets its acceptance criteria.

## Verification

- `just format`: passed.
- `PATH=/home/nevd/go/bin:$PATH just lint`: passed, including Go lint (0
  issues), Markdownlint, and yamllint.
- `just test`: passed.
- `just test-pty`: passed.
- `just build`: passed.
- Docker/runtime checks ran with escalation.
