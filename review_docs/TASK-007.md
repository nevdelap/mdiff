# TASK-007 Review

**Task:** Complete Micro plugin compatibility within the two-pane model  
**Reviewer:** Rufus  
**Review pass:** 2  
**Decision:** APPROVED

## Findings resolved

### Micro's `TryBindKey` plugin API did not persist bindings

**Status:** Resolved in pass 2

The `micro/config` import now exposes Micro's persistent `TryBindKey`
behavior. The fixture checks that startup leaves a missing `bindings.json`
untouched before the plugin requests a binding, then verifies the binding is
active and persisted after the request.

Evidence: [`TryBindKeyPlug`](/home/nevd/mdiff/internal/micro/action/bindings.go:272),
[`luaImportMicroConfig`](/home/nevd/mdiff/internal/prototype/plugins.go:63),
and
[`TestPluginTryBindKeyPersistsOnlyWhenRequested`](/home/nevd/mdiff/internal/prototype/plugins_test.go:178).

### Plugin `deinit` was skipped when opening an initial pane failed

**Status:** Resolved in pass 2

Startup error paths now call `app.Close`, which runs plugin `deinit` before
closing plugin timers and restoring the terminal. A fixture forces an initial
pane-open failure and verifies the plugin's `deinit` callback runs.

Evidence: [`NewWithFiles`](/home/nevd/mdiff/internal/prototype/app.go:176)
and
[`TestPluginDeinitRunsWhenInitialPaneOpenFails`](/home/nevd/mdiff/internal/prototype/plugins_test.go:235).

## Review

Plugin discovery and initialization, Micro-compatible imports and callbacks,
focused-pane edits, diff updates, persistent bindings, normal file/process
effects, and two-pane layout adaptations meet TASK-007's acceptance criteria.
No open findings remain.

## Verification

- `just format`: passed.
- `PATH=/home/nevd/go/bin:$PATH just lint`: passed, including Go lint (0
  issues), Markdownlint, and yamllint.
- `PATH=/home/nevd/go/bin:$PATH just test`: passed.
- `PATH=/home/nevd/go/bin:$PATH just test-pty`: passed.
- `PATH=/home/nevd/go/bin:$PATH just build`: passed.

Docker/runtime access for lint was obtained through escalation.
