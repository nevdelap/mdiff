# Roles

## Usage

This document defines the implementer and reviewer roles for implementation
tasks. Each task should have one implementer who completes the scoped work and
one reviewer who independently checks it. The roles may be performed by separate
people or agent sessions.

The implementation plan points to active tasks and
`design_docs/future_work.html` records open follow-up work. Both roles work from
the same written task specification. Do not expand or reduce a task's scope
during implementation; record a planning change first when the specification
needs to change.

## Igor — implementer

Igor implements the next unblocked task in the implementation plan. Igor:

- follows the task's goal, scope, dependencies, acceptance criteria, and
  verification requirements;
- makes the necessary source, test, and documentation changes within that scope;
- runs the applicable `just` gates and records the exact commands and results;
- keeps the project buildable at the task's completion point; and
- reports missing or conflicting requirements instead of silently changing the
  task definition.

For Go changes, the standard quality gates are `just lint`, `just test`, and
`just build`. Run only the gates relevant to the final change when the task
specification permits a narrower set, and record any gate that could not run. Do
not weaken or remove a regression test to make a gate pass.

Igor does not write the reviewer's findings or mark the work reviewed.

## Rufus — reviewer

Rufus independently reviews the complete task diff against its specification,
surrounding code, and repository conventions. Rufus checks correctness, scope,
maintainability, test coverage, documentation, and the implementer's reported
verification. The reviewer reruns the applicable gates when the environment
allows it and records the commands and results.

Rufus inspects tests for weakened assertions, removed coverage, suppressed
failure output, arbitrary sleeps, or altered inputs that avoid the behavior
under test. These are material findings unless there is evidence that the change
addresses a harness-only issue without hiding a product defect.

Rufus records findings in `review_docs/<task-id>.md`, updating that document on
later passes. Findings identify the evidence, impact, and required resolution.
If material findings remain, the task stays in review. When all findings are
resolved and applicable checks pass, Rufus records approval and marks the task
complete in the implementation plan. Move any remaining follow-up to
`design_docs/future_work.html` and remove the finished task and review
documents; the repository history retains them.

Rufus does not modify implementation source or tests while reviewing. If a fix
is needed, Rufus returns the finding to Igor.

## Handoff

The next dependent task starts after its dependencies are complete. While a task
is active, its implementation, review record, and plan state should agree. The
detailed task states and review procedure are in
`design_docs/agent_workflow.md`.
