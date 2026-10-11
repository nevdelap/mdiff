# New Tasks Specification Review

**Reviewer:** Rufus\
**Review pass:** 4\
**Scope:** `TASK-003` through `TASK-010` in the implementation plan

## Decision

All findings from passes 1 through 3 are addressed. The task specifications are
ready for implementation, subject to their stated dependencies and verification
gates.

No task state was changed. The implementation plan remains the source of truth.

## Findings

### 1. High — TASK-005 requires configuration behavior before TASK-006

**Status:** Addressed in pass 2

`TASK-005` says the shortcut bar must reflect configured keybindings. Its
dependencies include `TASK-004`, but user `bindings.json` support is introduced
by `TASK-006`, which depends on `TASK-005`. The current order therefore either
pulls configuration work into TASK-005 or leaves the shortcut-bar criterion
unmet at the end of TASK-005.

**Required resolution:** Keep TASK-005's shortcut bar tied to built-in/effective
bindings available at that stage, then add user-remapped shortcut coverage in
TASK-006; alternatively move the configuration-dependent shortcut-bar work to
TASK-006. Preserve the existing dependency direction and state the split in both
task scopes and acceptance criteria.

Evidence: [`TASK-005`](/home/nevd/mdiff/design_docs/implementation_plan.md:404)
and [`TASK-006`](/home/nevd/mdiff/design_docs/implementation_plan.md:490).

### 2. Medium — TASK-003 refers to an unspecified style precedence

**Status:** Addressed in pass 2

TASK-003 requires added/deleted character styles to preserve syntax foreground
and selection/cursor precedence “as specified by the renderer,” but the design
does not define an ordering among syntax, line-level hunk style, character
change style, selection, and cursor. Different choices change the visible
result, especially when selections cross changed text.

**Required resolution:** Add an explicit style-composition order and fallback
behavior to TASK-003 or the renderer design before implementation. Include a
test for a selected changed span and for the cursor on a changed cell.

Evidence: [`TASK-003`](/home/nevd/mdiff/design_docs/implementation_plan.md:259)
and the renderer behavior in
[`micro_diff_editor.md`](/home/nevd/mdiff/design_docs/micro_diff_editor.md:695).

### 3. Medium — TASK-004 leaves the large-paste debounce rule ambiguous

**Status:** Addressed in pass 2

TASK-004 sets a default debounce of 80 ms and says it is configurable only
within 40–180 ms, then allows a “large-paste adjustment up to 200 ms.” It does
not define how a user configures the delay, what size qualifies as a large
paste, or whether that adjustment is a cap or an extension of the user value.
Those choices affect deterministic timer tests and the input-responsiveness
contract.

**Required resolution:** State the configuration source and valid range, the
large-paste threshold and adjustment rule, and when the timer starts/restarts.
Keep the resolved maximum explicit in the acceptance tests.

Evidence: [`TASK-004`](/home/nevd/mdiff/design_docs/implementation_plan.md:331).

### 4. Medium — TASK-009 has no measurable memory or fallback-time pass limit

**Status:** Addressed in pass 2

TASK-009 calls for performance and resource targets and requires adversarial
fallback “within the configured bound,” but the plan supplies no fallback
deadline or peak-memory ceiling. It asks the report to observe memory and allows
platform tooling to vary, so a result could be recorded without a measurable
resource acceptance decision. The design has concrete latency targets, but no
corresponding maximum RSS target.

**Required resolution:** Specify the reference environment and a measurable
fallback deadline and peak-memory target, or explicitly define resource
acceptance as structural bounds plus recorded measurements and require an owner
decision for any observed regression. Fix the exact generated input parameters
used for the adversarial cases.

Evidence: [`TASK-009`](/home/nevd/mdiff/design_docs/implementation_plan.md:704)
and
[performance targets](/home/nevd/mdiff/design_docs/micro_diff_editor.md:879).

### 5. Low — TASK-010 asks for reproducibility without an acceptance check

**Status:** Addressed in pass 2

TASK-010's scope asks to exercise release-build reproducibility and its handoff
asks for release build hashes, but its acceptance criteria only require
release-build CI jobs to pass. They do not say how reproducibility is checked or
which release artifacts and build inputs are compared.

**Required resolution:** Define the reproducibility check (for example, two
builds from the same revision, toolchain, and flags producing identical
artifacts), identify the artifacts/platforms in scope, and record the command
and result in the handoff. If byte-identical builds are not a goal, remove the
reproducibility claim and state what the hashes attest to.

Evidence: [`TASK-010`](/home/nevd/mdiff/design_docs/implementation_plan.md:806).

### 6. Medium — TASK-009's memory-gate workload is not pinned

The new exact generators specify three adversarial comparisons, but the 512 MiB
RSS limit applies to a separate “100,000-line / 10 MiB total-input” workload.
Its left/right line counts, line lengths, and content distribution are not
specified. The same nominal line and byte counts can produce materially
different diff and rendering allocations.

**Required resolution:** Add an exact deterministic generator for the RSS-gated
workload, including the split between panes, line count per pane, line-length
distribution, and content relationship. Identify that fixture by name in both
the benchmark and the acceptance criterion.

Evidence: [`TASK-009`](/home/nevd/mdiff/design_docs/implementation_plan.md:744)
and its RSS acceptance criterion at
[`implementation_plan.md`](/home/nevd/mdiff/design_docs/implementation_plan.md:784).

**Status:** Addressed in pass 3. The `rss-10mib-100k-balanced-near-equal-v1`
fixture now pins line counts, line lengths, pane contents, exact byte counts,
and content relationship; the RSS criterion names that fixture.

### 7. Medium — TASK-003's syntax-color wording needed alignment with renderer precedence

**Status:** Addressed in pass 4

The earlier wording implied that syntax foreground might remain on changed
cells, while the cell-style composition rule specifies that character-diff
styles override syntax foreground there. The current TASK-003 directs the
implementation to use that explicit composition order, resolving the behavior
ambiguity. Its renderer tests cover syntax colors, selected changed spans, and
the cursor on a changed cell.

**Resolution:** TASK-003 links to the defined cell-style composition order.
Syntax foreground is retained at the base/hunk layers; character styles override
it on changed cells, with selection and cursor applied afterward. No further
spec change is required.

Evidence: [`TASK-003`](/home/nevd/mdiff/design_docs/implementation_plan.md:288)
and
[`Cell style composition`](/home/nevd/mdiff/design_docs/micro_diff_editor.md:715).

## Task-by-task readiness

- **TASK-003:** Ready. It links to the defined style-composition order and its
  tests cover changed styles, syntax colors, selection, and cursor precedence.
- **TASK-004:** Debounce configuration, large-paste thresholds, and timer tests
  are now specified.
- **TASK-005:** The built-in-only shortcut bar is separated from user binding
  remaps, which TASK-006 owns.
- **TASK-006:** Ready. Its configuration tests explicitly verify that the CLI
  debounce option overrides the mdiff settings-file value.
- **TASK-007:** Ready. The handoff names the compatible transient-buffer plugin
  and representative incompatible plugin categories.
- **TASK-008:** Mapping, anchor, interaction, and platform criteria are
  sufficiently detailed for implementation after its stated dependencies.
- **TASK-009:** Fallback deadline, memory ceiling, and RSS-gated fixture are now
  specified.
- **TASK-010:** Byte-for-byte reproducibility now has an explicit procedure and
  acceptance criterion.

## Review basis

Read the current implementation plan, `micro_diff_editor.md`,
`agent_workflow.md`, `docs/roles.md`, and the existing pre-task and TASK-001/
TASK-002 review records. `design_docs/lessons_learned.md` does not exist in the
current workspace. This is a specification review; no implementation tests or
quality gates were run. Pass 2 confirmed the five initial findings were
addressed. Pass 3 confirmed the exact TASK-009 RSS fixture was specified and
flagged a TASK-003 wording mismatch. Pass 4 confirms TASK-003 now points to the
explicit style-composition order, resolving that mismatch. All seven recorded
findings are addressed. Markdownlint and the pinned `mdformat --check` on this
review document passed after formatting.
