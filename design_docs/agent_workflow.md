# Agent Workflow

This document describes the task and review workflow for `mdiff`. Role
responsibilities are in [`docs/roles.md`](../docs/roles.md). The implementation
plan is the source of truth for task scope, dependencies, acceptance criteria,
and task state.

## Task definition

Before implementation, add a complete task to the implementation plan. It must
include:

- a goal describing the outcome;
- dependencies and any required prior decisions;
- an explicit scope, including affected packages, documentation, and supported
  platforms where relevant;
- acceptance criteria that can be checked; and
- verification commands appropriate to the changes.

An implementation task should be self-contained enough for an implementer to
finish and an independent reviewer to assess without relying on undocumented
conversation context. If implementation uncovers a real design decision or scope
gap, record it and resolve the plan before proceeding with dependent work.

## Task states

Use these states in the implementation plan:

- `NEW`: specified and ready to implement once dependencies are complete;
- `IMPLEMENTED`: implementer has completed the change and recorded verification;
- `REVIEW`: independent review has open findings or review is in progress;
- `COMPLETED`: review is approved and acceptance criteria are met; or
- `BLOCKED`: work cannot proceed until a named dependency or decision is
  resolved.

Only the human project owner changes a blocked task back to `NEW`. A reviewer
does not mark a task complete while material findings remain.

## Implementation and review

1. Igor selects the first `NEW` task whose dependencies are `COMPLETED`.
2. Igor reads the task and applicable lessons, implements its scope, and runs
   the specified gates. For each new change with material code changes, Igor
   increments mdiff's patch version once and records it in the release notes,
   following the policy in [`docs/development.md`](../docs/development.md).
3. Igor records changed areas, verification commands and results, and any
   environment limitation in the task handoff. The task moves to `IMPLEMENTED`.
4. Rufus reviews the full change independently, checks the acceptance criteria,
   and runs the applicable gates where possible. Rufus records findings in
   `review_docs/<task-id>.md`; the task moves to `REVIEW` if findings remain.
5. Igor resolves findings without changing the task's scope. Rufus reviews the
   full updated change and updates the same review document.
6. After approval, Rufus records the final decision and marks the task
   `COMPLETED`. Dependent work can then begin.

Review documents should include the task identifier, review pass, decision,
findings with evidence, and verification commands and results. Preserve prior
findings as addressed or open on later passes; do not erase review history.

## Repository and change workflow

This project uses Jujutsu where `.jj/` is present. Check the repository root
before repository operations and use `jj` for status, history, diffs, and change
management. Do not add Git-specific manual steps when Jujutsu provides the same
operation. `git` may still be used by tools that require it internally.

Do not create commits unless the task or project owner explicitly asks for them.
Keep each task's changes reviewable and scoped. Before handoff, inspect the
final diff and report the current repository state when the metadata is
available. If repository metadata is read-only or a command is unavailable,
report that limitation rather than substituting an unrequested workflow.

## Verification

Use `just` as the documented interface to project commands. For a normal Go
implementation task, run the relevant subset of:

```sh
just format
just lint
just test
just build
```

Use `just check` for the complete local quality, test, and build gate when the
task calls for it. Formatting and lint-only documentation changes can use
`just format` and `just lint`; do not claim tests passed if they were not run.
CI runs the repository's configured Go version and operating-system matrix.

If a gate rewrites files, inspect the resulting diff and rerun the affected
check on the final contents. Report external requirements such as Docker,
network access, or installed tools when they prevent a gate from running.

## Regression integrity

Treat a failing test as evidence to investigate. Fix a product defect when the
implementation violates the intended behavior. Do not weaken assertions, remove
coverage, suppress useful failure output, add arbitrary retries, or change
inputs just to turn a failing test green. If the test contract itself is
ambiguous, resolve that in the task specification before changing the test.

Add durable lessons from completed tasks to
[`lessons_learned.md`](lessons_learned.md) when the project has accumulated
reusable findings.
