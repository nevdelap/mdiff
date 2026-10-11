# TASK-001 Review

**Task:** Pin Micro and audit prototype feasibility\
**Reviewer:** Rufus\
**Review pass:** 1\
**Decision:** APPROVED

## Findings

No material findings.

## Review

The handoff and updated Micro design satisfy TASK-001's Phase 0 acceptance
criteria:

- Micro is pinned to `v2.0.15` / `6a62575bcfdf4965f187eedafceb3400316e612b`. The
  design records the module path, toolchain, selected package closure, license
  files, runtime embed requirements, local patch inventory, and update
  procedure.
- The import mapping removes the upstream `internal/` path component and rehomes
  those packages beneath `mdiff/internal/micro/`. The documented temporary probe
  compiled all 13 selected packages, and the host visibility probe demonstrates
  the Go `internal` boundary. I independently reran the 13-package `go build`
  successfully.
- The design traces the transient buffer hazards through construction, save,
  backup/recovery, serialization, config startup, plugin callbacks, normal
  shutdown, and panic recovery. It identifies the required no-write constructor
  and finalizer changes and a test plan; it does not claim that stock Micro's
  scratch buffer is sufficient.
- The selected tcell key, paste, mouse-wheel, resize, and simulation-screen
  paths are documented, including the simulation screen's paste limitation and
  the need for PTY coverage.
- The GO recommendation is appropriately conditional on TASK-002 proving the
  shared-row renderer and transient no-write behavior before broader
  integration.

## Verification

- Reproduced the focused Micro rehome `go build` in
  `/tmp/mdiff-micro-rehome-probe`: passed.
- Recounted the documented focused closure: 74 non-test Go files and 20,876
  lines: passed.
- `GOCACHE=/tmp/mdiff-go-cache go vet ./...`: passed.
- `GOCACHE=/tmp/mdiff-go-cache GOLANGCI_LINT_CACHE=/tmp/mdiff-golangci-cache /tmp/mdiff-gobin/golangci-lint run --timeout=5m`:
  passed, 0 issues.
- `UV_CACHE_DIR=/tmp/mdiff-uv-cache UV_TOOL_DIR=/tmp/mdiff-uv-tools uv tool run --from mdformat==0.7.22 --with mdformat-tables==1.0.0 mdformat --check README.md docs design_docs`:
  passed.
- The same pinned `mdformat --check` command on `review_docs/TASK-001.md`:
  passed after formatting the new review record.
- The handoff reports `just format` and `just lint` passed. I could not rerun
  those complete recipes: Docker access is denied in this environment, blocking
  the yamlfmt, Markdownlint, and yamllint containers. The initial `just format`
  attempt also needed `GOCACHE=/tmp/mdiff-go-cache` because the default Go cache
  is read-only; with that override, Go formatting and mdformat ran before the
  Docker step failed.
- `jj diff` could not snapshot the working copy because `.git/objects` is
  read-only. No Git fallback was used. The handoff says the task changed only
  design documents and used temporary probes outside the product tree.
- No application tests were run; TASK-001 makes no application changes.

## Subsequent workspace checks

After this TASK-001 review, the workspace included Go code under
`internal/micro`, `internal/compare`, and `internal/viewport`; those files are
outside TASK-001's documented-only scope. At the user's request, I reran the
Docker-backed project recipes against the current workspace:

- `GOCACHE=/tmp/mdiff-go-cache XDG_RUNTIME_DIR=/tmp just format`: passed. Its
  `gofmt` step reformatted `internal/micro/action/bufpane.go`,
  `internal/micro/action/infocomplete.go`, `internal/micro/buffer/buffer.go`,
  `internal/micro/buffer/buffer_test.go`,
  `internal/micro/buffer/eventhandler.go`,
  `internal/micro/buffer/line_array.go`, `internal/micro/config/plugin.go`,
  `internal/micro/config/plugin_installer.go`,
  `internal/micro/config/settings.go`, `internal/micro/display/statusline.go`,
  and `internal/viewport/layout.go`.
- `GOCACHE=/tmp/mdiff-go-cache GOLANGCI_LINT_CACHE=/tmp/mdiff-golangci-cache XDG_RUNTIME_DIR=/tmp just lint`:
  failed at `go vet ./...`. The current root `go.mod` declares Go 1.19, while
  `internal/compare/alignment.go` uses `max`, `min`, and integer-range syntax
  requiring newer language versions. Vet also reported copied-lock and
  unkeyed-literal warnings in the current Micro subtree. The recipe stopped
  before golangci-lint.
- Direct `just lint-markdown` and `just lint-yaml` Docker recipes: passed.

These later workspace results do not change TASK-001's review decision: the
reported lint failure is in source outside TASK-001, and its own documentation
checks passed. The current workspace does need the Go version/lint failures
addressed before its full lint gate can pass.
