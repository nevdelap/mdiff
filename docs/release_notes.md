# Release notes

## Unreleased

### v0.0.6

- Restore Micro behavior parity for `QuitAll`, diff-key fallbacks, page actions,
  command parsing, help buffers, `SaveAll`, wheel bindings, scrollbars, and
  plugin reload policy.
- Keep the shared diff gutter as mdiff's replacement for Micro's VCS gutter.

### v0.0.5

- Share Micro's source-position calculation between cursor lookups and diff
  rendering, including tabs, wrapping, and combining characters. Keep selection
  and changed-character styling aligned across visual rows.

### v0.0.4

- Audit Micro's blocked built-in commands. Pass `eval` through to bundled
  Micro's unsupported message; keep `tabswitch` and commands that require extra
  views or an unsafe runtime reload visibly blocked.

### v0.0.3

- Restore Micro's interactive shell prompt and configured suspend action with
  coordinated terminal handoff, output pause, and two-pane redraw on return.

### v0.0.2

- Build the initial diff from file contents before the first draw so multiline
  files align and scroll correctly without an edit.
- Forward `just run` arguments safely, including flags and paths with spaces.
- Expand comparison and startup regression coverage and add race and fuzz CI
  checks.
- Preserve Micro movement, file, command, mouse, and raw key bindings while
  adapting diff navigation, shared scrolling, and two-pane file workflows.
- Honor Micro history, buffer state, key menu, and status line settings.
- Create Micro's selected default config directory on first run so configured
  history and buffer state can persist; explicit `--config-dir` paths must
  exist.
- Keep pane buffers synchronized when prompts finish after focus changes, and
  retain Micro pane ownership across terminal resize.
- Complete raw and mouse bound quit actions in the application event loop.
- Show bundled Micro help topics in a scrollable overlay with search and match
  navigation, including regular expressions, Micro's `ignorecase` setting, and
  matches that cross visual wraps.
- Verify alignment and changed-character rendering for repeated, reordered,
  Unicode, and separated changes in startup files.
- Render wide-character and long prompts, status, and help lines by visual
  columns, and honor Micro's configured fake cursor without an extra mark.
- Use Micro's command registry for command-bar dispatch, with explicit mdiff
  exclusions for commands that require unsupported views or terminal handoff.
- Keep prompts visible at two terminal rows, show the active help-search cursor,
  and wrap Unicode help text with matching highlight positions.
- Highlight tab matches across every help row they occupy. Apply the approved
  dark diff palette, readable light-theme fallbacks, and optional Micro
  colorscheme overrides; leave equal rows and themes with a terminal-default
  background in Micro's normal colors, with divider markers for diff rows on
  terminal-default themes.

`mdiff` opens two independently editable Micro buffers in one side-by-side
terminal session. It supports initial file arguments, per-pane Open and Save,
Micro-compatible editing and configuration, character-level change highlighting,
diff navigation, aligned scrolling, wrapping, and terminal resize.

The project is licensed under the same MIT license text as Micro. The vendored
Micro source and third-party notices remain under `internal/micro/`.

### Known limitations

- Micro Lua plugins are trusted code. They are not sandboxed and can access the
  filesystem, processes, network, and Micro configuration.
- Plugins and keybindings that require extra tabs, panes, independent vertical
  scrolling, or unavailable Micro host operations are unsupported.
- Micro's tcell dependency remains pinned at v2.0.13 because v2.1 and newer
  remove APIs required by the vendored Micro integration.
- Configured Micro backup and autosave settings can write document artifacts;
  ordinary exit does not save unsaved buffers without an explicit save choice.
- Linux and macOS release verification runs in the GitHub Actions build and
  release jobs; release hashes are recorded in the TASK-010 handoff.
