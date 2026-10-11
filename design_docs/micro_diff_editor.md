# mdiff: Technical Design

**Status:** Proposed\
**Date:** 2026-10-11\
**Scope:** Architecture and implementation plan; no application code is
specified by this document as complete.

## Executive summary

`mdiff` is a standalone Go terminal application with two empty, editable text
buffers. It borrows Micro's buffer, action, syntax, settings, keybinding, and
Lua plugin behavior, while replacing the ordinary split-pane vertical scrolling
model with one diff-aware visual coordinate system.

Micro is not currently a clean embeddable editor library. Its buffer and editor
packages are under Go's `internal/` tree; its normal program is assembled
through global configuration, screen, tab, and pane state; and its renderer
assumes that a pane can derive its own scroll position from its own buffer. The
preferred integration is therefore a maintained, locally namespaced Micro source
subtree under `mdiff/internal/micro/`, with a small set of explicit hooks for
in-memory buffers, aligned rendering, and shared viewport control. This retains
real Micro editing behavior and avoids a process boundary, while keeping Micro
changes reviewable and upstream synchronization possible. The initial prototype
must test this choice before the rest of the design is committed.

For line diffing, use Micro's already-declared
`github.com/sergi/go-diff/diffmatchpatch` dependency at line granularity. It is
an established Go implementation already used in Micro's buffer code. Convert
its operation stream into an immutable alignment map; do not use Micro's
existing per-buffer diff gutter as the alignment model. A debounced,
single-flight worker computes from immutable snapshots and drops stale results.

The central invariant is that the panes never own independent vertical offsets.
A shared alignment map describes paired logical lines and virtual spacer rows.
Each paired row expands into as many terminal rows as the taller wrapped line
requires. The shared viewport is an offset into this expanded sequence. Each
renderer maps that offset back to a real buffer line, a wrap subrow, or a blank
spacer. Edits and diff replacement preserve a viewport anchor by remapping its
logical line/context into the new alignment map.

The project should adapt `stay`'s `just`-driven workflows, documentation
quality, explicit development gates, `uv`-managed support scripts, and
Jujutsu-first repository handling. Rust-specific Cargo, Rust toolchain, Nix
module, and nextest practices do not transfer directly. Go operations use the Go
toolchain (`go test`, `go vet`, `gofmt`, and a pinned Go linter) behind `just`
recipes. `difftastic` is the preferred human diff viewer where available; no
Git-specific workflow is required.

## Goals and non-goals

### Goals

- Launch `mdiff` with no arguments into two empty, editable panes.
- Preserve Micro editing behavior for cursor motion, selections, undo/redo,
  indentation, syntax highlighting, configured bindings, and practical
  compatible Lua plugins.
- Load the user's Micro settings, keybindings, color schemes, and plugin
  directory where their behavior is compatible with this application.
- Accept terminal-native paste in either focused pane and retain independently
  editable buffers.
- Continuously show line-level additions, deletions, and replacements, using
  virtual rows for unmatched lines without changing either text buffer.
- Keep both panes aligned under scrolling, cursor navigation, difference
  navigation, mouse wheel input, wrapping, and terminal resize.
- Keep all document data in memory. With plugins disabled, mdiff never saves or
  recovers buffer contents from disk on exit. Explicitly enabling trusted Lua
  plugins opts out of the guarantee for plugin side effects.
- Remain responsive for large paste operations and avoid holding multiple
  avoidable full copies of the text.
- Restore terminal state after normal exit, handled signals, and returned
  errors.
- Establish a `stay`-inspired, Go-native build, lint, format, test, and CI
  workflow.

### Non-goals

- Opening, reloading, saving, or saving as a file.
- A merge tool, three-way comparison, filesystem watcher, or Git/Jujutsu
  integration in the product.
- A complete reimplementation of Micro or support for arbitrary non-Micro
  plugins.
- Perfect compatibility with plugins that assume a disk-backed file, normal
  Micro process lifecycle, command bar, tabs, arbitrary panes, or unrestricted
  filesystem access.
- Syntax-aware or semantic diffing in the initial release. Diff equality is
  based on exact line content, including whitespace.

## Investigation method and confidence

The local `~/stay` checkout was inspected on 2026-10-11: its README, `justfile`,
`docs/development.md`, design documents, lint configuration, quality dispatcher,
release workflow, CI, package manifest, and source/test layout. This checkout
has Jujutsu metadata. Its written guidance says to use `jj` for repository
operations when working in that repository; that convention is recorded here as
a project convention, not applied to Micro research.

Micro source was audited from release `v2.0.15`, commit
`6a62575bcfdf4965f187eedafceb3400316e612b`, on 2026-10-11. The module proxy
reported this exact origin for tag `refs/tags/v2.0.15`; the module path inside
the release remains `github.com/zyedidia/micro/v2` although the repository is
now `micro-editor/micro`. The audit covered the buffer, action, display, screen,
configuration, Lua/plugin, runtime, and terminal dependency paths listed below.
Source links in this document resolve to the pinned commit.

Statements labeled **Verified** describe inspected upstream source or `stay`
files. **Proposal** identifies design choices, expected behavior, and
modifications that have not yet been implemented or experimentally validated.

## Existing `stay` conventions to adapt

The following observations are verified in `~/stay`:

- `justfile` is the main developer interface. It has discoverable recipes, quiet
  wrappers that put full output in `check.log`, narrow and whole-tree scopes,
  build/run/test gates, and explicit release/maintenance commands.
- `docs/development.md` describes a normal local loop, small relevant gates,
  full handoff gates, acceptance tests, and treating a passing result as valid
  only for the snapshot actually checked.
- Markdown is formatted with `mdformat` using an 80-column wrap setting and
  linted with Markdownlint; other file families use a quality dispatcher.
  `uv run --script` runs Python support scripts without adding a project Python
  environment.
- CI installs tools explicitly, runs quality checks, tests, release builds,
  dependency audits, acceptance checks, and platform-specific checks. Job
  timeouts are based on observed runtime.
- Jujutsu is the intended repository interface in a checkout that has `.jj/`;
  the instructions explicitly avoid direct Git commands for repository
  operations. `difftastic` is a useful human-readable diff viewer in the user's
  requested toolset.
- `design_docs/` holds durable technical and implementation planning;
  `docs/development.md` is the user/maintainer workflow manual. The existing
  implementation plan is task-oriented and points to shared workflow and role
  documents.

### Go translation

**Proposal:** `just` remains the front door. `just build` and
`just build-release` wrap `go build`; `just test` wraps the complete Go test
suite; `just test-package` and `just test-filter` provide targeted loops;
`just vet`, `just lint`, `just format`, `just check-fast`, and `just check` wrap
idiomatic Go tools. Use a pinned `golangci-lint` (or a small explicit linter
set) and `gofmt`; do not add a Rust-style MSRV or copy Cargo/nextest commands.
The supported Go version should be the project's declared `go` directive and CI
matrix floor.

Keep `go.mod`/`go.sum` checked in, use `go mod tidy` as a reviewed maintenance
operation, and run `govulncheck` or an equivalent dependency audit in CI. `uv`
remains suitable for repository-local support scripts. Document how to run
CI-equivalent commands locally. Keep quality output in `check.log` via a quiet
Just wrapper. Use `jj` for manual repo status/history/diff operations when the
mdiff checkout has `.jj/`; tool invocations may internally use Git only where an
existing configured tool requires it. Prefer `difft` for reviewing patches when
installed.

**Open convention decision:** `stay` packages releases with Nix and Home
Manager, but this request excludes Rust-specific aspects and asks only for the
design. Whether mdiff should inherit Nix packaging is a product/release choice;
it is not required to build this Go design and should be decided during project
bootstrap.

## Micro implementation findings

### Verified facts

- `internal/buffer/buffer.go` defines `Buffer`, `SharedBuffer`, `LineArray`,
  cursors, edit state, syntax highlighter references, and undo event handling.
  `Buffer.Insert` and `Buffer.Remove` delegate to `EventHandler`; buffers can be
  created from strings. `SharedBuffer.MarkModified` re-highlights changed lines.
- The same file defines `BTScratch`, but its `Fini` path in the inspected source
  calls `Serialize()` for a non-shared, unmodified buffer before cancelling
  backups. Scratch semantics alone therefore do not prove the requested
  no-persistence property. A dedicated transient buffer type and a guarded
  finalizer are required.
- Micro buffer construction initializes settings from `config.GlobalSettings`,
  applies filetype and path-glob rules, may apply backups, initializes cursors
  and plugin callbacks, and adds to a global `OpenBuffers` collection.
  Disk-oriented setup and teardown must be deliberately bypassed or patched for
  two in-memory buffers.
- `internal/action/bufpane.go`'s `BufPane` connects a `*buffer.Buffer` to a
  display window and owns the editor action dispatch. It exposes `DoKeyEvent`,
  `DoMouseEvent`, and editor actions are registered by names from keybinding
  configuration. `internal/action/actions.go` contains cursor, selection,
  indentation, editing, and split-related actions.
- `internal/action/tab.go` builds tabs and split trees. `VSplitBuf` creates
  another ordinary `BufPane`; `Tab` dispatches input to the active pane, sends
  mouse-wheel input to the pane under the pointer, and resizes panes from split
  geometry. This is a useful precedent for two editing panes, but it does not
  establish shared diff-aware vertical scrolling.
- `internal/display/bufwindow.go`'s `BufWindow` embeds a `View` with `StartLine`
  and `StartCol`, calculates its own dimensions, wraps long lines, draws syntax
  styles and the normal diff gutter, and runs `Relocate()` against that pane's
  cursor and own `StartLine`. `displayBuffer()` renders one buffer at a time.
  The ordinary panes therefore have independent view state.
- `internal/screen/screen.go` initializes a tcell screen, enables configured
  paste and mouse handling, supports a simulation screen for tests, and exposes
  `Fini` through tcell on shutdown paths. The app lifecycle must consistently
  call that cleanup.
- `internal/config/config.go` resolves configuration using `MICRO_CONFIG_HOME`,
  then `XDG_CONFIG_HOME/micro`, then `~/.config/micro`; it creates the config
  directory. `settings.go` loads options. The runtime includes default
  keybindings, syntax definitions, and color schemes. Plugin handling is
  implemented in Go and invokes Lua callbacks; plugin docs describe callbacks
  receiving concrete buffer pane objects.
- Micro's `go.mod` includes `github.com/sergi/go-diff/diffmatchpatch`. Its
  `Buffer` includes built-in diff status/gutter concepts, but these are
  per-buffer display hints and do not constitute a two-buffer aligned row map.

### Phase 0 audit at the pinned revision

**Verified pin and toolchain:** Micro `v2.0.15` is pinned to commit
`6a62575bcfdf4965f187eedafceb3400316e612b`. Its `go.mod` declares
`github.com/zyedidia/micro/v2` and Go `1.19`; the `mdiff` Go `1.26.0` module can
consume this source. The manifest has 18 direct and 9 indirect requirements and
two replacements (`go-shellquote` and `gopher-luar`). The focused package graph
resolves Micro's 11 internal packages (`action`, `buffer`, `clipboard`,
`config`, `display`, `info`, `lua`, `screen`, `shell`, `util`, `views`), plus
`pkg/highlight` and `runtime`, and their Go dependencies. `action` pulls in
`shell`, so the full upstream internal package tree is part of the current
focused closure; this is not a small buffer-only extraction.

**Verified compile probe:** the above 11 packages, `pkg/highlight`, and
`runtime` were copied to `/tmp/mdiff-micro-rehome-probe` from the pinned module
cache. In that temporary copy only, imports were rewritten from
`github.com/zyedidia/micro/v2/internal/...` to
`github.com/nevdelap/mdiff/internal/micro/...`, and from the upstream `pkg/` and
`runtime` paths to their corresponding `internal/micro/` paths. With the
upstream dependencies and `go.mod` otherwise retained, Go 1.26.8 successfully
ran `go build` on all 13 packages. This verifies that the focused source closure
can compile with the proposed package rehoming and module identity. It does not
verify an mdiff app entrypoint, plugin startup, or modified renderer. The probe
used no files in the mdiff product tree and did not modify the upstream module
cache.

The source audit used this exact module download command (with temporary caches
outside the repository):

```sh
GOMODCACHE=/tmp/mdiff-micro-gomodcache GOCACHE=/tmp/mdiff-go-cache \
  go mod download -json github.com/micro-editor/micro/v2@v2.0.15
```

The actual package closure was listed from the downloaded source directory with:

```sh
GOMODCACHE=/tmp/mdiff-micro-gomodcache GOCACHE=/tmp/mdiff-go-cache \
  go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' \
  ./internal/buffer ./internal/action ./internal/display ./internal/screen \
  ./internal/config | sort -u
```

The rehomed source compile probe was run in the disposable copy with:

```sh
GOMODCACHE=/tmp/mdiff-micro-gomodcache GOCACHE=/tmp/mdiff-go-cache \
  go build ./internal/micro/action ./internal/micro/buffer \
  ./internal/micro/clipboard ./internal/micro/config \
  ./internal/micro/display ./internal/micro/info ./internal/micro/lua \
  ./internal/micro/screen ./internal/micro/shell ./internal/micro/util \
  ./internal/micro/views ./internal/micro/pkg/highlight \
  ./internal/micro/runtime
```

**Verified source volume and license obligations:** the focused production
closure contains 74 non-test Go files and 20,876 source lines, before counting
runtime data. Micro is MIT licensed. Preserve its root `LICENSE` and
`LICENSE-THIRD-PARTY` verbatim with the copied subtree; retain copyright and
license headers on copied files. Inventory licenses for the Go modules actually
distributed and include their notices in releases. `internal/micro/runtime` must
retain the embedded `colorschemes`, `help`, `plugins`, and `syntax` directory
structure because `runtime/runtime.go` uses `//go:embed` relative to that
package. Record the pin and local patch list next to the subtree, and update by
reviewing a candidate tag/SHA, regenerating the dependency/import inventory,
reapplying the small patch set, and rerunning the compile and test gates before
replacing the pin.

**Verified transient-buffer paths:** `BTScratch` sets `Scratch: true`, which
causes `SharedBuffer.setModified` to return before dirty tracking or backup
requests, and `Buffer.SaveAs` rejects scratch buffers. It also sets
`Syntax: false`, so it does not meet the editor requirement. A proposed
`BTTransient` can use a distinct kind with `Scratch: true`, `Syntax: true`, and
an empty `Path`. With an empty path, `ApplyBackup` and `Unserialize` skip disk
data, `Serialize` returns before encoding or writing, `Backup` and
`RemoveBackup` return before filesystem operations, and autosave sees the
scratch buffer as unmodified. `Buffer.Fini` still calls `Serialize` and queues
`CancelBackup`; the new type should instead take an explicit no-write finalizer
path. `SaveAs` rejects scratch types before `saveToFile` reaches file creation,
but user-facing save actions should still be filtered to avoid a filename
prompt.

**Verified constructor and application hazards:** even with `Path == ""`,
`NewBuffer` computes an absolute path, applies path-glob settings, calls
`UpdateModTime` (which stats the empty path), unconditionally stats and may
create `ConfigDir/buffers`, conditionally attempts `Unserialize`, invokes the
`onBufferOpen` plugin callback, and adds the buffer to global `OpenBuffers`. For
`BTTransient`, patch construction to retain an empty virtual path and skip
path-glob, backup/recovery, modtime, serialized-state, and `buffers/` directory
work. Set the display name after construction. The expected application config
root may still be created by `InitConfigDir`; that is a config-directory side
effect, not a document-content write.

Micro's `cmd/micro` startup also calls `checkBackup` for settings and bindings;
that helper can restore files with `os.WriteFile`. `action.InitBindings` creates
a missing `bindings.json`. `config.WriteSettings`, plugin installation, the
stock `exit`, and the panic-recovery path can write files or back up buffers.
The mdiff entrypoint must bypass these startup/exit paths, read user config
without creating default files, filter file actions, and use its own idempotent
shutdown path. `config.ReadSettings` and `config.InitGlobalSettings` read and
initialize settings in memory; `config.InitRuntimeFiles(true)` reads user
runtime files and embedded defaults. `InitConfigDir` creates the config
directory if absent.

**Verified plugin and Lua constraint:** `config.InitPlugins` discovers
`init.lua` and files under `plug/`; `config.LoadAllPlugins` executes them. The
Lua bridge exposes `io`, `os`, `net/http`, and filesystem helpers, so plugins
are unsandboxed trusted code. The normal `lua.L` initialization is in
`cmd/micro/initlua.go` (a `package main` file), not the reusable internal
packages. However, buffer/pane constructors and callbacks unconditionally call
`luar.New(ulua.L, value)` even when the plugin list is empty; `luar.New` needs a
non-nil Lua state for pointer arguments. The no-plugin app still needs to
initialize an empty Lua state or patch those callback adapters to avoid Lua
conversion when plugins are disabled. Plugin support later needs to move the
Micro Lua bridge initialization behind an explicit mdiff-owned API. TASK-001's
product policy remains plugins disabled by default and explicit opt-in for the
trusted, no-write-exempt plugin mode.

**Verified terminal/event route:** `screen.Init` initializes tcell, applies the
configured paste and mouse settings, and must be paired with `Screen.Fini`.
`BufPane.HandleEvent` handles `EventKey`, `EventPaste`, and `EventMouse`;
`EventPaste` calls the normal paste insertion path. `TabList.HandleEvent` routes
`EventResize` to layout resize. `Tab.HandleEvent` finds the pane under a mouse
wheel event and forwards it; default Micro bindings map wheel up/down to
`ScrollUp`/`ScrollDown`. `InitSimScreen` returns the fork's
`tcell.SimulationScreen`, which supports injected key and mouse events and
resizing. Its `SetPaste` is a no-op and the interface has no paste injector;
simulation tests can dispatch `tcell.NewEventPaste` directly to the pane, while
a PTY test must send bracketed-paste control sequences to verify terminal
parsing and restoration. The app must send resize events through the event
dispatcher after changing simulation dimensions.

**Verified diff API:** Micro uses `github.com/sergi/go-diff` `v1.1.0`.
`DiffMatchPatch.DiffLinesToRunes(text1, text2)` returns two rune slices and a
shared line table; `DiffMainRunes(left, right, false)` computes operations on
the line tokens with line checking disabled; `DiffCharsToLines` restores line
text. `DiffTimeout` defaults to one second and is configurable on the diff
object. The API accepts no `context.Context` and returns no timeout status, so
mdiff needs bounded worker ownership and must not interpret timeout completion
as an error signal. These operations are a suitable prototype baseline, but must
be benchmarked for adversarial large inputs in Phase 4.

**Proposed local patch inventory:** (1) rehome all focused Go packages under
`internal/micro` and rewrite the legacy upstream module imports; (2) keep
embedded Micro runtime assets under `internal/micro/runtime`; (3) introduce
`BTTransient`, skip its constructor disk/config-state paths, and make `Fini`
strictly no-write; (4) initialize buffer and display globals explicitly from
mdiff, including a Lua state for callback value conversion without loading
plugins; (5) split read-only default keybinding initialization from the upstream
helper that creates `bindings.json`; (6) guard or replace independent
`BufWindow.Relocate`/`displayBuffer` behavior with the shared visual-row
provider; and (7) keep upstream notices and pin/patch metadata with the subtree.
This is a proposal for TASK-002 and later work; no upstream or product source
was modified for this audit.

**Recommendation: GO to TASK-002, with a strict prototype gate.** The pinned
source closure compiles after rehoming, and its real buffers, key dispatch,
tcell, and rendering code can be used without a separate process. The closure is
large (20,876 Go source lines) and the renderer currently owns independent
`StartLine` state, so this does not justify a full integration commitment. TASK-
002 should validate the shared-row renderer before any expansion into full
configuration or editing behavior. Stop and reassess the fork approach if the
prototype must replace most of `BufWindow` to render shared rows.

### Consequences

A separate process running stock `micro` cannot satisfy the integration
contract: its buffers, actions, panes, styles, and renderer are not exported as
a stable embedding API, and terminal output from another process cannot provide
accurate virtual alignment. A new TUI built from scratch would reproduce
substantial editor behavior and still would not load Micro Lua plugins
compatibly.

A Go module cannot import `github.com/zyedidia/micro/v2/internal/...` from an
unrelated module. Copying or vendoring those files under the original import
path does not remove the internal import rule. The fork must either keep mdiff's
host code within Micro's module path (undesirable as a public module identity)
or move copied packages beneath mdiff's module-level `internal/` directory and
rewrite their imports. This design chooses the latter. The copied top-level
`internal` path component must be removed: placing a package at
`mdiff/internal/micro/internal/buffer` would make it importable only from within
`mdiff/internal/micro`, excluding the host at `mdiff/internal/app`.

**Verified Go import-layout probe:** a temporary module using mdiff's module
path, with a package at `internal/micro/buffer` imported by `internal/app`,
passes `GOCACHE=/tmp/mdiff-go-cache go test ./...` on Go 1.26.8. This validates
the proposed Go visibility boundary. The separate source rehome probe above also
validates the focused Micro dependency closure.

## Investigated alternatives

| Alternative                                                                                           | Benefits                                                                                                                                                                    | Costs and risks                                                                                                                                                                       | Decision                          |
| ----------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- |
| Launch stock Micro twice and synchronize terminal panes                                               | No source fork; immediately uses Micro binaries                                                                                                                             | No safe buffer API; no shared input/edit state; terminal scraping cannot reliably recover logical lines, selections, or wrap geometry; cannot add virtual rows or guarantee alignment | Reject                            |
| Build mdiff around public Micro packages only                                                         | Smallest copied surface if sufficient APIs exist                                                                                                                            | Buffer/actions/splits/config integration live under `internal`; exported highlighting package alone is insufficient; plugins expect Micro concrete runtime and pane objects           | Reject as full solution           |
| Write a new Go editor UI and translate Micro settings                                                 | Clean mdiff architecture and easier unified scrolling                                                                                                                       | Reimplement editing, syntax, undo, key dispatch, plugin API; translation cannot provide real Micro plugin compatibility                                                               | Reject for primary path           |
| Fork Micro as its own module and change its main program                                              | Maximum compatibility with Micro internals, plugin lifecycle, and terminal renderer                                                                                         | Product becomes a long-lived downstream editor fork; custom binary and renderer modifications are deep; source and module layout couple mdiff to Micro                                | Feasible, prototype fallback      |
| Keep a Micro source subtree under `mdiff/internal/micro`, rewrite import prefix, and adapt entrypoint | Real editor core in-process; internal imports satisfy Go rules; app has explicit ownership of render/input lifecycle; a maintained patch series can be reviewed and rebased | Initial subtree migration and import rewrite; upstream sync burden; globals and callbacks require careful reset/ownership; renderer must be modified                                  | **Recommend, gated by prototype** |

Selective extraction is appropriate only if the prototype demonstrates that a
small dependency closure can provide genuine editing and compatibility. Current
source coupling suggests the display/action/config/plugin combination is larger
than a reasonable independent extraction. Do not claim that copying only
`pkg/highlight` provides Micro editor reuse.

## Recommended architecture

### Process and package layout

Proposed layout, adjusted to actual repo conventions at bootstrap:

```text
cmd/mdiff/main.go                 # signal setup, CLI, exit code
internal/app/                     # lifecycle and event loop
internal/micro/                   # namespaced Micro source fork and patches
internal/compare/                 # snapshots, diff result, alignment map
internal/viewport/                # shared expanded-row viewport and anchors
internal/terminal/                 # event routing and terminal session wrapper
internal/config/                   # mdiff-specific flags/policy over Micro config
internal/testpty/                  # PTY harness for terminal-level tests
runtime/mdiff/                     # only mdiff-owned defaults, if needed
scripts/                           # quality, docs, and CI support scripts
justfile
go.mod
design_docs/
docs/development.md
```

The subtree may be smaller if the phase-one dependency audit proves extraction
safe. Keep Micro code in its own clearly marked subtree with upstream
version/commit, license notices, local patch list, and a documented update
procedure. Rehome upstream Go packages using this mapping:

| Upstream path                                  | mdiff path                                        |
| ---------------------------------------------- | ------------------------------------------------- |
| `internal/buffer`                              | `internal/micro/buffer`                           |
| `internal/action`                              | `internal/micro/action`                           |
| `internal/display`                             | `internal/micro/display`                          |
| `pkg/highlight`                                | `internal/micro/pkg/highlight`                    |
| `github.com/zyedidia/micro/v2/internal/buffer` | `github.com/nevdelap/mdiff/internal/micro/buffer` |

The last row shows the import rewrite: remove the upstream top-level `internal/`
component and put each imported Micro package below
`github.com/nevdelap/mdiff/internal/micro/`. Rehome other upstream Go packages
needed by the copied dependency closure under that subtree. Keep non-Go runtime
assets in a separately documented runtime location or embed them as the source
audit requires. Do not leave Micro's top-level `internal` directory nested below
`internal/micro`, and do not publish Micro internals as a public Go API.

The Go `internal` boundary for `mdiff/internal/micro/buffer` is the module root
`github.com/nevdelap/mdiff`; therefore `github.com/nevdelap/mdiff/internal/app`
may import it, while unrelated modules may not. The checked-in `go.mod` module
path is `github.com/nevdelap/mdiff`. The pinned source closure and import
mapping above were compile-probed before any product source was copied.

`cmd/mdiff` calls an app constructor and owns signal handling. `internal/app`
owns exactly two transient buffers, two editor panes, one comparison worker, one
alignment map, one viewport, and one terminal session. Avoid Micro package
globals leaking across multiple app instances in tests; add an explicit reset or
dependency container for config, open buffers, screen, and plugin state.

### Lifecycle

1. Parse no-argument launch and mdiff-owned options such as `--help`,
   `--version`, `--config-dir`, `--enable-plugins`, and optional initial
   filetype. Reject file operands because opening files is out of scope.
2. Initialize Micro config paths and read existing options, bindings, schemes,
   and syntax definitions without modifying user configuration or creating
   document backups. Do not load plugins unless `--enable-plugins` was
   explicitly supplied. If config initialization creates the normal Micro
   configuration directory, that side effect is the existing Micro behavior; do
   not write buffer data there.
3. Initialize tcell and install a deferred, idempotent `Fini` path before
   starting the event loop. Register signal handling for interrupt and
   termination signals; cancellation exits through the same cleanup. Avoid
   attempting unsafe terminal work from signal handlers.
4. Construct two empty `BTTransient` buffers. Set independent display names such
   as `Left` and `Right`; allow optional filetype override to configure syntax
   rules without presenting a disk path. Initialize a normal Micro `BufPane`
   around each.
5. Set both panes' vertical view adapter to the shared viewport controller.
   Configure a two-column layout and a status bar with focus and diff state.
6. Render, route terminal events, apply edits through Micro actions, schedule
   comparison after content changes, and install only results whose versions
   match both current buffers.
7. On quit or signal, stop comparison workers, disable timers, close transient
   buffers through a no-write teardown path, call tcell `Fini`, and return. No
   serialization, backup, save prompt, recovery file, or stdout buffer flush is
   allowed.

### Buffer ownership and editing

Each side owns one distinct Micro `Buffer`, cursor set, undo/redo event history,
selection, filetype, syntax state, and settings overlay. The comparison engine
receives immutable content snapshots only; it never mutates either buffer. A
monotonically increasing content version accompanies every buffer mutation,
including undo/redo and paste.

Add a `BTTransient` or equivalent constructor in the Micro fork with these
invariants:

- No filesystem path is opened, statted, watched, or used for recovery.
- No autosave, backup, serialized cursor/undo state, save prompt, file reload,
  or document write is reachable from the normal action map.
- `Fini` drops in-memory state and cancels any timers without calling
  `Serialize`, `RequestBackup`, or file operations.
- The buffer remains syntax-enabled and editable; `BTScratch`'s current
  syntax-disabled settings are not a substitute.
- Each buffer has independent undo/redo and cursor state; shared text state is
  never used across sides.

Interpose on the Micro mutation/event-handler boundary so every text change
increments a version and sends a coalescible
`BufferChanged{side, version, changedRange}` notification. Do not infer changes
by polling cursor position or relying only on per-frame `ModifiedThisFrame`,
which is reset by rendering and is not a stable change stream.

### Configuration and plugin compatibility

Read the standard Micro config directory and standard file formats. Do not copy
Micro defaults over user files or write mdiff-only settings into
`settings.json`; mdiff-specific overrides belong in command flags or a
namespaced `mdiff.json`/TOML file. Preserve Micro's option values and keybinding
action names where actions exist. Use synthetic buffer names and an explicit
optional filetype to get syntax behavior without file I/O.

| Micro feature                  | Planned behavior                                                                                                                          | Compatibility limit                                                                                                                                                                                                                                                                                            |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `settings.json` global options | Load and apply options relevant to buffers, terminal colors, mouse, paste, and rendering                                                  | Options for tabs, file reload, backup, save, shell, or unrelated panes are ignored or forced safe                                                                                                                                                                                                              |
| `bindings.json`                | Load Micro buffer bindings; retain built-in editing actions                                                                               | Actions that open files, save/write, run commands, suspend the app, or depend on ordinary splits are disabled or remapped                                                                                                                                                                                      |
| `colorschemes/`                | Load selected scheme and use Micro's syntax and UI styles                                                                                 | mdiff adds a documented small set of diff-specific style names, with sensible fallback when absent                                                                                                                                                                                                             |
| `syntax/`                      | Use Micro syntax definitions and filetype matching                                                                                        | Runtime syntax files work for synthetic names/filetype override; file-on-disk detection is absent                                                                                                                                                                                                              |
| Lua plugins and `plug/`        | Disabled by default; load compatible plugins only with the explicit `--enable-plugins` opt-in and compatible Micro lifecycle/action hooks | Enabled plugins are trusted code with the application's filesystem and process privileges. Their effects are outside mdiff's no-write guarantee; display a persistent warning while they are enabled. Plugins that save/reload/open files, assume one active tab, or control split offsets remain unsupported. |
| Micro commands                 | Retain safe editing commands and command-line action behavior where feasible                                                              | `save`, `saveas`, `open`, `reload`, and shell/file commands are removed or return a clear unsupported message                                                                                                                                                                                                  |
| Micro clipboard                | Terminal bracketed paste is supported; Micro's configured clipboard integration may be used for explicit copy/paste actions               | External clipboard tools vary by OS and plugin; terminal paste is the required baseline                                                                                                                                                                                                                        |

Arbitrary Lua plugins can perform effects outside the buffer API. The product
guarantee is bounded: mdiff's built-in behavior never writes, serializes, backs
up, or recovers either document, but explicitly enabled Lua plugins are trusted
code and can access filesystem or process APIs. The whole-process no-write
guarantee does not cover plugin side effects. Plugins are disabled by default
and require `--enable-plugins` on each launch; do not enable them solely because
the user's Micro configuration has plugins installed. When enabled, show a
persistent warning that plugin code can write buffer contents or other files.
Never describe plugins as sandboxed. A future restricted plugin mode requires a
separate design and security audit.

### Diff computation

Use `diffmatchpatch` from the Micro dependency already in its `go.mod`. Encode
lines into tokens with the package's line-diff helpers, compute a diff on token
streams, then decode operations. Confirm exact helper names and limits against
the pinned version during implementation. Treat trailing newline and final empty
line semantics explicitly; the snapshot layer must match Micro's `LineArray`
representation, not `strings.Split`'s accidental edge behavior.

For each operation block:

- Equal: pair lines one-to-one as `Equal` rows.
- Delete followed by insert (or insert followed by delete): pair
  `min(deleted, inserted)` lines as `Changed` rows to show replacements
  together; emit `LeftOnly` or `RightOnly` rows for the remainder with a virtual
  spacer on the opposite side.
- Pure insertion/deletion: emit actual lines on one side and virtual spacer rows
  on the other.

The map references logical line indices and operation type. A spacer is a
rendering record only; it is never inserted in `Buffer.LineArray`, affects
cursor positions, enters undo, or gets included in snapshots. Provide
next/previous-difference navigation from the changed block index, with wrap
policy documented in the UI.

On mutation, wait a short configurable debounce (proposal: 80 ms, bounded to
40–180 ms), cancel a queued older job when possible, capture immutable snapshots
plus versions, and run at most one comparison at a time. On completion, discard
a result if either version has changed. If a newer change arrived during
computation, immediately schedule the newest snapshot. Render the last valid
diff during computation, mark it stale in the status bar, and avoid flashing all
lines as changed. A large-paste threshold may lengthen the debounce up to 200
ms; it must not delay initial feedback indefinitely.

Initially recompute the full line diff. This is simpler and less error-prone
than incremental Myers edit maintenance. The engine interface should allow
replacing the implementation if profiles justify an incremental strategy. No
worker may access live mutable Micro buffers concurrently.

### Alignment map and shared visual coordinate

A logical alignment map consists of ordered blocks. Each block contains one or
more `AlignedLine` records; each record identifies an optional line on the left,
optional line on the right, a change kind, and a rendered height. An absent line
means a virtual blank row.

```go
type Side uint8
const ( Left Side = iota; Right )

type ChangeKind uint8
const ( Equal ChangeKind = iota; Changed; LeftOnly; RightOnly )

type AlignedLine struct {
    LeftLine  int // -1 means virtual spacer
    RightLine int // -1 means virtual spacer
    Kind      ChangeKind
    Height    int // max wrapped display rows for either real line, minimum 1
}

type DiffSnapshot struct {
    LeftVersion, RightVersion uint64
    Rows []AlignedLine
    Prefix []int64 // cumulative expanded visual-row offsets, len(Rows)+1
    Hunks []Hunk
}

type Viewport struct {
    TopVisualRow int64
    Height       int
    LeftStartCol int
    RightStartCol int
}
```

The *canonical vertical coordinate* is the integer expanded visual-row index in
`DiffSnapshot`, not a logical line number, not either Micro `StartLine`, and not
a row in one pane. A paired logical row's height is
`max(leftWrappedRows, rightWrappedRows)`; virtual spacers have one row. If one
real line wraps into three terminal rows and its paired line wraps into one,
both panes occupy three visual rows for that alignment record. The shorter side
renders blank continuation rows. This intentionally delays its next logical line
to retain horizontal correspondence.

`Prefix[i]` maps alignment row `i` to its first visual row. Binary search maps
`TopVisualRow` back to an alignment record and subrow in `O(log R)`; rendering
the visible screen costs `O(H)` rows plus visible cell rendering. Heights can be
recomputed after width/option changes in `O(R)`; prefix rebuilding then costs
`O(R)`. A Fenwick/segment tree is a later optimization if measured edit
workloads need incremental height changes.

Each side retains its own horizontal `StartCol`. If soft wrap is enabled,
horizontal offsets are zero and both sides use the same terminal-row coordinate.
If wrapping is disabled, every aligned record has height one, and horizontal
scrolling clips each side independently; vertical alignment still holds. In the
initial release, `softwrap` remains on by default as Micro config specifies; a
user's `softwrap=false` is honored. Gutter widths are accounted for separately
when calculating wrap widths, so pane column widths need not be numerically
identical.

The renderer replaces or adapts the per-pane calls to `BufWindow.Relocate()` and
`displayBuffer()`: ordinary `Relocate()` cannot independently move `StartLine`
after every action. The adapter maps `TopVisualRow` into each pane's visible
start line/subrow before drawing. It asks Micro to render real lines, syntax,
selections, whitespace indicators, and cursors through a row-provider or forked
draw loop that accepts `AlignedLine` and subrow. It renders a spacer as a
cleared row with diff gutter marker and the configured
`diff-deleted`/`diff-added` style; no fake text is put in the buffer.
Replacement text highlighting can use line-level background and optional
intraline character highlights as a later enhancement.

### Shared viewport, cursor movement, and scroll anchoring

All vertical movement goes through `SharedViewport`:

- Wheel input over either pane changes `TopVisualRow` by a configured
  terminal-row amount and redraws both panes.
- Keyboard scroll actions from either focused pane change the same offset.
- Cursor navigation and selection actions execute normally on the active
  `BufPane`, then map that cursor's logical line and wrapped subrow through the
  alignment map. If it falls above or below the visible range (including scroll
  margin), move the shared offset just enough to bring it into view. The other
  pane moves with it.
- Jump-to-next/previous-difference selects the target hunk, maps its first row
  to a visual coordinate, and places it near a stable viewport position
  (proposal: one quarter from the top when a jump is far away). Cursor focus
  remains on the chosen side unless the user explicitly switches panes.
- Clicks on a virtual spacer do not create a buffer cursor. The click is routed
  to the nearest real line in that pane or ignored according to a deterministic
  rule; clicking a real aligned row focuses that pane and maps x/y to its real
  buffer location.
- Exactly one terminal cursor is visible, in the focused pane. Inactive-pane
  cursors do not compete for the terminal cursor. The active caret stays visible
  after resize and diff remapping.

When the diff map changes, preserve the top of the viewport using an anchor, not
the old integer row alone. Before installing a new map, capture the top visible
real line on the focused side (or both sides if focused line is a spacer), its
line index, the first visible wrapped subrow, and a small surrounding content
fingerprint. Use the old map to identify its aligned counterpart. After
recalculation, map unchanged line content/context to the nearest unambiguous
row; if the line itself was edited, use the mutation's old-to-new line delta and
then nearest following unchanged context. If neither side provides a stable
match, anchor to the active cursor's aligned row. Keep the same subrow where
valid and clamp the new top to the new document bounds. This handles an
insertion above the viewport without jumping to the same absolute line number.
Repeated identical lines can make fingerprints ambiguous, so tie-break by
nearest prior logical index and report this residual risk for
property/interaction testing.

Do not animate alignment changes in the first release. Install one complete
immutable map at a render boundary, move `TopVisualRow` through the anchor
mapping, then redraw both panes in the same frame. This avoids a frame where one
side uses new alignment and the other uses old alignment.

### Resize and wrap changes

On terminal resize, recompute the split geometry and each pane's text width
after gutter/status deductions. Recalculate wrapped row counts for visible and
total lines, rebuild the height prefix map, remap the viewport anchor, and
ensure the focused cursor is visible. For large documents, calculate line
display widths lazily and cache by
`(bufferVersion, lineIndex, width, tabSize, wrapMode)`; invalidate affected
lines on edits and all line heights when pane width or wrapping settings change.

Micro's current `BufWindow` derives horizontal and vertical behavior from its
own dimensions and cursor. The fork must make shared viewport positioning
authoritative after resize and after every action that calls `Relocate`. Avoid
calling ordinary relocation on both panes and then trying to reconcile the
results; that produces cursor jumps around spacer rows.

## Component boundaries and interfaces

The following signatures are illustrative; they are not promises about exact
Micro upstream APIs.

```go
type BufferSnapshot struct {
    Side    Side
    Version uint64
    Lines   [][]byte // immutable line references or copied bytes
}

type DiffEngine interface {
    Compare(ctx context.Context, left, right BufferSnapshot) (*DiffSnapshot, error)
}

type ChangeSource interface {
    Changes() <-chan BufferChanged
    Snapshot(side Side) BufferSnapshot
}

type Aligner interface {
    Build(left, right BufferSnapshot, ops []DiffOp, widths Widths, opts RenderOptions) *DiffSnapshot
    LocateLine(diff *DiffSnapshot, side Side, line int) (visualRow int64, subrow int, ok bool)
    LocateVisualRow(diff *DiffSnapshot, visualRow int64) (row int, subrow int)
}

type SharedViewport interface {
    ScrollRows(delta int64)
    EnsureCursorVisible(side Side, line, visualColumn int)
    Anchor(diff *DiffSnapshot) ViewAnchor
    Install(diff *DiffSnapshot, anchor ViewAnchor)
    Resize(widths Widths, height int)
}

type EditorPane interface {
    HandleEvent(tcell.Event)
    Buffer() *microbuffer.Buffer
    CursorLocation() microbuffer.Loc
    SetAlignedViewport(AlignedViewport)
    Render(AlignedRows, Viewport)
}

type App interface {
    Run(ctx context.Context) error
    Close() error // idempotent; stops workers, closes transient buffers, restores terminal
}
```

`Compare` owns line tokenization and hunk grouping but not buffer
synchronization. `Aligner` owns logical-to-visual mapping and wrap heights.
`SharedViewport` owns one vertical coordinate and two horizontal offsets.
`EditorPane` adapts real Micro actions/rendering to the diff row provider. `App`
coordinates lifecycle, events, and stale results.

## Event flows

### Paste or edit

1. tcell emits paste or key input.
2. The app identifies the pane from event coordinates or current focus. The pane
   sends it through Micro's regular event/action pipeline, preserving configured
   keybindings and undo grouping.
3. Mutation hook increments the side's content version and publishes a change
   range. Cursor relocation calls the shared viewport adapter.
4. The diff coordinator restarts the debounce timer. The old diff remains
   visible and is marked stale.
5. At expiry, the coordinator captures both immutable snapshots and submits one
   job.
6. The worker computes line operations and alignment records without reading
   live buffers.
7. The event loop receives a result. If versions differ, discard it. Otherwise
   anchor the current top into the new map, atomically install it, recalculate
   visible rows if necessary, and redraw both panes.

### Scrolling and cursor movement

1. Route event to the pane under the mouse or active pane for keyboard input.
2. For editor navigation, run the configured Micro action against that pane's
   real cursor.
3. Translate the cursor logical line and wrap subrow to canonical visual row.
4. Adjust shared `TopVisualRow` only if cursor is outside the viewport/scroll
   margin. For wheel or explicit scroll, apply the requested visual-row delta
   directly.
5. Draw both panes from the same snapshot and top coordinate; show terminal
   cursor only in the focused pane.

### Exit or interruption

1. Cancel event loop context and debounce timer; prevent new compare
   submissions.
2. Cancel or join the worker. A late result is ignored after shutdown.
3. Run `Close` once: call transient buffer cleanup and terminal `Fini` even if
   plugin cleanup returned an error.
4. Return a concise error to `main`; do not emit buffer text or run save
   serialization.

## Error handling and shutdown

Configuration parse failures should identify the file and either follow Micro's
documented fallback behavior or stop when the chosen scheme/bindings cannot be
applied; report this per option. A malformed plugin or plugin callback failure
should be logged/reported and should not panic the event loop. Diff worker
errors leave the previous alignment map active and surface a status message.
Terminal initialization errors return before raw mode can remain active. All
teardown functions are idempotent and run under `defer` after successful
initialization.

Signal handling should use `signal.NotifyContext` for `SIGINT`, `SIGTERM`, and
platform-supported equivalents. The normal event loop handles cancellation and
executes cleanup. Do not call tcell or buffer APIs from an asynchronous signal
callback. If an uncatchable termination occurs, terminal restoration cannot be
guaranteed; this is an operating-system limit.

## Performance and memory targets

Targets are proposals to validate on a documented reference machine and
terminal:

- Small input up to 10,000 lines: settled diff update within 100 ms after the
  debounce on typical edits.
- 100,000 lines / 10 MiB total input: keep keystroke/paste event handling
  responsive; settled diff within 1 s for a near-equal edit and within 2 s for a
  large replacement.
- 1 MiB paste: UI should begin rendering the accepted text within 250 ms; diff
  may remain marked stale while the worker runs.
- No unbounded queue of snapshots or compare jobs; at most one active and one
  latest pending request.
- Alignment metadata should be `O(N+M)`; line snapshots should avoid repeated
  whole-document byte copies when a safe immutable snapshot can be obtained. Any
  required copy is bounded to one active snapshot pair.
- Rendering work should be proportional to visible terminal rows and cells, not
  all lines. Width/height map rebuild may be `O(N+M)` initially; measure before
  adding a tree structure.

Line diff normally scales with input plus edit distance using Myers-style
matching; worst-case behavior can be quadratic in total line count. The chosen
`go-diff` implementation has timeout/algorithm settings that must be configured
and measured for line-token inputs. A timeout must produce a usable coarse
fallback (for example, mark the affected region as changed and show a status
warning) rather than freezing the UI. Benchmark repeated lines, all-different
input, a single-line edit, and very long lines. Do not run syntax highlighting
or diff computation synchronously in terminal input handling for a large paste.

## Testing strategy

### Unit tests

- Snapshot semantics for empty buffers, final newline, CRLF policy, empty final
  line, Unicode, tabs, and invalid byte policy.
- Diff operation conversion: equality, insertion, deletion, replacement pairing,
  consecutive hunks, duplicate lines, and no underlying buffer mutation.
- Visual mapping round trips: logical line/subrow to visual row and reverse;
  prefix boundaries; spacer rows; clamping at start/end.
- Wrap-height calculation for tabs, wide runes, combining marks, exact-width
  lines, soft wrap, horizontal scroll, and width changes.
- Viewport anchor behavior for insertions/deletions before, at, and after the
  top row; edited anchor line; duplicate-line ambiguity; hunk removal; anchor on
  spacer; cursor fallback.
- Debouncer single-flight behavior, version matching, stale result drop,
  cancellation, and shutdown races with deterministic fake clock/worker
  interfaces.
- Transient buffer tests proving that close/exit does not call save, serialize,
  backup, reload, or create document files; undo/redo and syntax remain active.
- Configuration precedence and compatibility policy; no user configuration
  writes.

### Property and fuzz tests

- Generate arbitrary line sequences, compute operations, and assert replay
  reconstructs both sides exactly.
- Assert every real line on each side occurs exactly once in the alignment
  stream, spacers occur only opposite an actual unmatched line, and order is
  monotone.
- Assert `LocateLine` and `LocateVisualRow` agree for all generated rows and all
  wrap heights >= 1.
- Fuzz snapshots with newline edge cases, Unicode, large repeated runs, and
  empty input. Bound CPU/memory for adversarial generated sequences and ensure
  cancellation is honored.

### Integration tests

- Use Micro's tcell simulation screen (verified upstream test support in
  `screen.InitSimScreen`) to instantiate two empty buffers, paste/edit both,
  switch focus, invoke configured bindings, undo/redo, and inspect cell output
  and cursor position.
- Validate both pane outputs against the same canonical visual sequence after
  edits and scroll actions.
- Load temporary Micro config fixtures for settings, bindings, scheme, syntax,
  and a compatible plugin. Assert incompatible save/open actions are unavailable
  and fixture files remain unchanged.
- Exercise resize and different pane widths with softwrap on/off; confirm cursor
  remains visible and paired logical rows remain aligned.

### Terminal interaction tests

Use a PTY harness analogous to `stay/tests/helpers/acceptance_pty.bash`, with
Bats or Go PTY support selected at bootstrap. Launch the built `mdiff`; send
bracketed paste to left and right, key navigation, focus switch, wheel events
where the PTY/terminal protocol permits, diff navigation, resize, Ctrl-C, and
normal quit. Capture terminal bytes/screen snapshots, verify alternate
screen/mouse/bracketed-paste modes are restored, process exits, and no content
or recovery file is created. Keep Linux and macOS acceptance runs because tcell
and terminal behavior varies by OS.

## Build, development, lint, format, and CI

**Proposal, adapting `stay`:**

- `just` is the documented command interface, with `just --list` as the help
  entry point.
- `just install` installs project hooks if adopted and downloads Go module
  dependencies; it must not silently install arbitrary system packages.
- `just build`, `just build-release`, `just run`, `just test`,
  `just test-package`, `just vet`, `just lint`, `just format`, `just audit`,
  `just check-fast`, and `just check` are explicit and documented.
- Formatting uses `gofmt` and `mdformat`; Markdown lint settings follow the
  local 200-character prose/table convention only if retained by mdiff.
  YAML/Justfile formatting follows dedicated formatters. Keep file selection
  explicit in a small quality dispatcher if the repo grows to multiple
  languages.
- Quiet recipes write full logs to a task-local `check.log` and print a short
  tail on failure. Format recipes must verify they leave the selected snapshot
  clean.
- Tests use Go's standard runner and race detector. Add `go test ./...`,
  `go test -race ./...`, `go vet ./...`, pinned `golangci-lint`, and dependency
  audit to CI. Avoid introducing Rust-specific nextest or toolchain logic.
- CI should test supported Linux and macOS targets at minimum, build an
  optimized binary, run unit/integration tests, run PTY acceptance where
  supported, and audit dependencies. Derive timeouts from observed runtimes as
  `stay` does.
- `docs/development.md` should explain local prerequisites, Just recipes, Go
  version policy, quality gates, how to refresh the Micro subtree, how to review
  a snapshot with `jj` and `difft`, and how to capture PTY diagnostics.
- Use `uv` for Python helper scripts and `difftastic` for local review where
  present. Do not make either mandatory to build or run mdiff.

## Implementation phases and gates

### Phase 0: Pin and audit (complete for TASK-001)

The audit is recorded above for Micro `v2.0.15` at
`6a62575bcfdf4965f187eedafceb3400316e612b`, including the focused dependency
closure, temporary source-rehome compile probe, license obligations, transient
buffer persistence paths, plugin boundary, terminal events, and diff API. The
recommendation is **GO to the bounded TASK-002 prototype**. The prototype must
still validate the adapted renderer and no-write behavior before any full
integration commitment.

### Phase 1: Integration and scroll prototype (highest priority)

Create a minimal branch/subtree prototype with two empty transient Micro
buffers, one tcell screen, one split renderer, simple line diff, virtual
spacers, synchronized scrolling, cursor visibility, bracketed paste, and resize.
Do not build full config/plugin support first. Use simulation screen plus PTY to
demonstrate a deletion on one side, insertion on the other, scroll from either
pane, softwrap with unequal widths, and cursor navigation across a hunk without
drift. **Exit gate:** both panes render the same canonical expanded visual rows
and pass anchor stability scenarios. If Micro rendering cannot be adapted
without replacing most of `BufWindow`, reassess a Micro fork as the primary
application rather than continuing a misleading package extraction.

### Phase 2: Buffer lifecycle and editor action preservation

Implement `BTTransient`, mutation version events, two independent undo
histories, Micro action dispatch, focus switching, paste, editing, indentation,
cursor/selection, and safe shutdown. Disable all built-in file operations.
**Exit gate:** integration tests prove ordinary editing, undo/redo, and no
content persistence.

### Phase 3: Config, syntax, and plugin compatibility

Load Micro settings, bindings, color schemes, syntax definitions, and plugin
hooks under the selected policy. Add safe action filtering, compatibility
diagnostics, and `--enable-plugins` opt-in with a persistent trust warning.
**Exit gate:** fixture tests and at least representative real user config/plugin
smoke tests; publish the support matrix.

### Phase 4: Diff, viewport, wrapping, and performance hardening

Replace prototype diff with versioned debounced worker; implement anchors,
navigation, wrap-height map, resize cache invalidation, large-paste behavior,
timeout fallback, and benchmarks. **Exit gate:** performance targets met or
revised with measured evidence; property/fuzz tests and multi-platform PTY suite
pass.

### Phase 5: Project quality and release readiness

Finalize Just recipes, `docs/development.md`, CI, dependency audits, licenses,
upstream sync notes, diagnostics, and distribution packaging. **Exit gate:**
clean full `just check`, Linux/macOS PTY acceptance, reproducible release build,
and documented Micro update process.

## Risks and mitigations

| Risk                                                        | Impact                                                                | Mitigation                                                                                                                                                     |
| ----------------------------------------------------------- | --------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Micro is not a stable embedding API and uses global state   | Integration and test isolation can become fragile                     | Keep a namespaced subtree, pin upstream, isolate globals behind app construction, prototype first                                                              |
| Go `internal` restriction blocks direct import              | A standalone module cannot import Micro internals                     | Rehome Micro packages from upstream `internal/<pkg>` to `mdiff/internal/micro/<pkg>` and rewrite imports; compile-probe host imports from `mdiff/internal/app` |
| `BufWindow` rendering assumes independent view offsets      | Independent scroll can undo alignment on every action                 | Replace relocation ownership with a shared viewport adapter and row provider; validate in Phase 1                                                              |
| Scratch close may serialize state                           | Data can be written on exit despite no Save command                   | Add a dedicated transient type and tests for every teardown path                                                                                               |
| Micro plugins can make arbitrary filesystem/process changes | A plugin may write buffer contents despite built-in no-write behavior | Disable plugins by default; require `--enable-plugins`, state that plugin side effects are outside the no-write guarantee, and show a persistent warning       |
| Micro update churn and local patch conflicts                | Maintenance cost and delayed security fixes                           | Pin SHA, keep patch series narrow, schedule upstream sync, run tests against candidate updates                                                                 |
| Diff worst-case runtime/memory                              | UI stalls after large paste                                           | Debounce, one worker, cancellation/version drops, thresholds, timeout fallback, benchmark adversarial inputs                                                   |
| Diff alignment can be ambiguous with repeated lines         | Jumps or unstable hunk movement                                       | Deterministic tie-break, contextual anchoring, preserve old anchor/cursor, test duplicate content                                                              |
| Two lines with unequal wrap heights complicate comparison   | Alignment can appear to insert content rows                           | Define pair height as max wrap height and draw continuation blanks; test all wrap cases                                                                        |
| Terminal variations in paste, mouse, resize, true color     | Different interaction behavior across terminals                       | Use tcell capabilities, simulation tests, Linux/macOS PTY matrix, explicit capability fallback                                                                 |
| Micro config includes commands that do not apply to mdiff   | Unexpected writes, broken bindings                                    | Filter unsupported actions with clear diagnostics and document limits                                                                                          |
| Upstream/embedded licenses and attribution are missed       | Distribution/legal defect                                             | Preserve Micro license/third-party notices, inventory vendored dependencies, include notices in releases                                                       |

## Unresolved questions

1. Is `softwrap` enabled by default for mdiff, or should mdiff override Micro's
   user value for stronger line alignment? This proposal respects the user's
   Micro value and handles variable heights explicitly.
2. Should changed blocks pair replacements line-by-line, or should all delete
   rows precede all insert rows? This proposal pairs by index to make
   side-by-side comparison easier; snapshot semantics must be stable and tested.
3. What initial size threshold should trigger coarser diff fallback or a
   warning? Benchmark in Phase 4 before fixing a hard limit.
4. Should Home Manager/Nix packaging be part of initial scope? `stay` supports
   it, but this design does not assume it.

## Acceptance criteria

The design is complete when implementation can be reviewed against all of the
following:

- `mdiff` is a standalone Go CLI and no-argument launch starts two empty
  in-memory editors.
- The recommended Micro integration has a concrete import-path strategy that
  respects Go `internal` restrictions, identifies required local patches, and
  preserves an upstream update path.
- Micro buffer ownership, action dispatch, configuration, syntax, keybinding,
  color, plugin, and persistence behavior have explicit boundaries and
  compatibility limits.
- The document defines a shared expanded visual-row coordinate that includes
  inserted/deleted spacer rows and wrap subrows; neither pane independently
  chooses vertical scroll.
- Edits, cursor movement, mouse/keyboard scrolling, diff jumps, resize,
  horizontal scroll, unequal wrap height, cursor visibility, stale async diffs,
  and viewport anchoring have deterministic rules.
- Diff results are immutable/versioned, stale work is dropped, computation is
  debounced/off the input path, and adversarial complexity has a fallback.
- A transient buffer lifecycle prevents save, backup, serialization, reload, and
  recovery writes for application-owned content.
- The test plan includes unit, integration, property/fuzz, simulation-screen,
  PTY paste/scroll/resize/interrupt, and no-write assertions.
- The plan has independent milestones and Phase 1 proves real Micro integration
  and diff-aware synchronized scrolling before full config/plugin commitments.
- Go-specific commands are used behind `just`; `stay`-applicable docs, quality,
  CI, `uv`, `jj`, and `difftastic` conventions are adapted without Rust-specific
  operations.
- Every unverified upstream assumption is labeled and tied to source paths;
  source links identify the relevant Micro files and docs.

## Upstream references

Micro source links below point to the audited immutable revision
`6a62575bcfdf4965f187eedafceb3400316e612b` (`v2.0.15`). The `go-diff` links are
upstream project references; Micro pins the dependency at `v1.1.0`.

- [Micro repository and README](https://github.com/micro-editor/micro)
- [Micro v2.0.15 release](https://github.com/micro-editor/micro/releases/tag/v2.0.15)
- [Micro `go.mod`](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/go.mod)
- [Micro license](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/LICENSE)
- [Micro third-party notices](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/LICENSE-THIRD-PARTY)
- [Buffer model and construction](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/buffer/buffer.go)
- [Buffer undo/event handling](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/buffer/eventhandler.go)
- [Buffer persistence and backups](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/buffer/serialize.go)
- [Buffer save and autosave paths](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/buffer/save.go)
- [Buffer backup worker and guards](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/buffer/backup.go)
- [Buffer pane actions and event dispatch](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/action/bufpane.go)
- [Editing actions](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/action/actions.go)
- [Tabs, splits, mouse routing, and active pane](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/action/tab.go)
- [Buffer rendering, independent view state, wrapping, gutters, and relocation](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/display/bufwindow.go)
- [Terminal screen lifecycle, paste/mouse setup, and simulation screen](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/screen/screen.go)
- [Micro config directory resolution](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/config/config.go)
- [Settings loading and option defaults](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/config/settings.go)
- [Runtime file and default resource loading](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/config/rtfiles.go)
- [Default and user keybinding setup](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/action/bindings.go)
- [Plugin runtime](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/internal/config/plugin.go)
- [Lua initialization in the stock entrypoint](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/cmd/micro/initlua.go)
- [Micro keybindings help](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/runtime/help/keybindings.md)
- [Micro plugin compatibility documentation](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/runtime/help/plugins.md)
- [Micro option reference](https://github.com/micro-editor/micro/blob/6a62575bcfdf4965f187eedafceb3400316e612b/runtime/help/options.md)
- [Micro `sergi/go-diff` upstream](https://github.com/sergi/go-diff)
- [`diffmatchpatch` line tokenization and diff implementation](https://github.com/sergi/go-diff/blob/master/diffmatchpatch/diff.go)
- [Go package API for `diffmatchpatch`](https://pkg.go.dev/github.com/sergi/go-diff/diffmatchpatch)
- [Alternative Go text diff package, `gotextdiff`](https://github.com/hexops/gotextdiff)
  (copied from gopls internals; API and maintenance ownership need review before
  selection)
