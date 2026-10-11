# Pre-task Planning Review

**Reviewer:** Rufus\
**Review passes:** 1–2\
**Scope:** Current project scaffold, implementation plan, technical design, and
role/workflow guidance, before TASK-001 implementation began.

## Project context

`mdiff` intentionally builds on the non-Rust parts of `~/stay`, including its
Just-driven workflow, documentation and review conventions, and repository
practices. The design translates those conventions to Go and explicitly leaves
Rust-specific tooling out. This is established project context, not a review
finding.

## Decision

At this planning review, TASK-001 was ready to start. The two findings from pass
1 were addressed in the task/design text; one scope clarification remains for
TASK-002 planning. TASK-001 was later implemented and reviewed separately in
[TASK-001 Review](TASK-001.md). This record does not change implementation-task
state.

## Findings

### Addressed in planning — High: proposed Micro import layout could violate Go

`internal` rules

The design proposes copying Micro beneath `mdiff/internal/micro/` and rewriting
imports to `github.com/<owner>/mdiff/internal/micro/...`
([design](../design_docs/micro_diff_editor.md#recommended-architecture)). If
Micro's existing `internal` directory is retained, a package such as
`mdiff/internal/micro/internal/buffer` is importable only by packages within
`mdiff/internal/micro`. The proposed host package `mdiff/internal/app` cannot
import it directly. Being within the same Go module does not remove this
restriction.

**Resolution:** The design now maps upstream `internal/<pkg>` packages to
`mdiff/internal/micro/<pkg>` and says to remove the upstream `internal/`
component from rewritten imports
([package mapping](../design_docs/micro_diff_editor.md#recommended-architecture)).
TASK-001 acceptance criteria require compile-probing that layout and checking
the selected dependency closure before copying source
([TASK-001](../design_docs/implementation_plan.md#task-001--pin-micro-and-audit-prototype-feasibility)).
The task still needs to produce that evidence; the plan is now specific enough
for Igor to do so.

### Addressed in planning — Medium: document-data guarantee and plugin behavior

needed an explicit boundary

The design goal says document contents are never saved or recovered to disk, but
the plugin section recognizes that Lua plugins can perform arbitrary file
operations and treats plugins as trusted code
([goals](../design_docs/micro_diff_editor.md#goals-and-non-goals),
[plugin policy](../design_docs/micro_diff_editor.md#configuration-and-plugin-compatibility)).
TASK-001 disables plugins for the prototype, which is appropriate, but it does
not settle how the product guarantee applies when plugins are eventually
enabled.

**Resolution:** The design now says plugins are disabled by default, require an
explicit `--enable-plugins` opt-in on each launch, and are trusted code whose
filesystem/process side effects fall outside mdiff's built-in no-write guarantee
([plugin policy](../design_docs/micro_diff_editor.md#configuration-and-plugin-compatibility)).
It also calls for a persistent warning when they are enabled. TASK-001 carries
that policy forward; full plugin compatibility remains deferred as intended.

### Planning clarification for TASK-002 — prototype and Phase 2 scope overlap

TASK-002 requires real Micro editing actions, bracketed paste, and transient
no-write teardown. The design's Phase 2 also lists transient buffers, action
dispatch, editing, and paste
([phases](../design_docs/micro_diff_editor.md#implementation-phases-and-gates)).
Before TASK-002 starts, specify which minimal behaviors belong to the disposable
prototype and which are production-hardening work for Phase 2. This does not
block TASK-001, but will help avoid implementing the same scope twice or leaving
an unclear prototype boundary.

## Planning note

The design describes phases for buffer lifecycle, configuration/plugins,
performance, and release readiness, while the implementation plan currently
specifies only TASK-001 and TASK-002. This is consistent with the stated
architecture gate: those later phases should be turned into scoped tasks after
TASK-002's decision, before work proceeds into them.

## Review checks

- **Pass 1:** Read `README.md`, `docs/development.md`, `docs/roles.md`,
  `design_docs/agent_workflow.md`, `design_docs/implementation_plan.md`, and
  `design_docs/micro_diff_editor.md`.
- **Pass 2:** Re-read the current scaffold and planning documents, including the
  revised import mapping, plugin policy, and TASK-001 acceptance criteria.
- No tests or quality gates were run; this remains a planning review, not an
  implementation review.
- `jj status` could not report the working-copy state because it attempted to
  write under the read-only `.git/objects` directory.
