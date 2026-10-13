# Release notes

## Unreleased

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
- Linux and macOS release verification is pending the TASK-010 GitHub Actions
  run for this revision. Supported-runner claims depend on those jobs passing.
