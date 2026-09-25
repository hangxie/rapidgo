# RapidGo TODO

Current baseline: PR #7 is merged into `main`. The terminal shell, project tree, and keyboard-operated editor are implemented. This list tracks work toward the [v0.1 acceptance test](docs/MVP.md); each numbered item is a reasonable PR-sized milestone or a small group of closely related PRs.

## Foundation

- [x] 0. Create the repository, document the MVP and architecture, add CI, and merge the Kong/Testify CLI foundation.
- [x] 1. Replace the CLI placeholder with a terminal application shell. Choose a TUI library, wire a single event loop, render a status bar and empty panes, restore the terminal on exit, and handle resize and narrow terminals. Keep terminal types inside `internal/ui`.

### PR #3 review

- [x] 1a. Handle SIGINT/SIGTERM independently of tcell's bounded event queue; test cancellation while that queue is full.
- [x] 1b. Keep help and quit shortcuts visible when a narrow terminal displays a long project name.
- [x] 1c. Add a keyboard-operable menu bar to the terminal shell with working File and Help actions.
- [x] 1d. Apply explicit VGA colors, gray menu/status/dialog surfaces, red shortcuts, framed dropdowns, and a pane title for project context.

## Browse and edit

- [x] 2. Add a project model and file tree. Open `rapidgo .` at an arbitrary directory, discover Go modules without requiring one, walk files without blocking the UI, and open a selected UTF-8 file. Cover inaccessible paths and large trees.
- [x] 3. Add a terminal-independent editor buffer. Support insertion, deletion, line breaks, cursor movement, selection, and undo/redo with defined UTF-8 positions. Test multi-byte, combining, and wide characters without depending on the TUI library.
- [x] 4. Connect the buffer to the editor pane. Provide non-modal keyboard editing, scrolling, focus changes between tree and editor, visible dirty state, and unsaved-change protection when switching files or quitting. Tree/editor focus and active/inactive borders are in place.
- [ ] 5. Add file save, in-file search, and `gofmt` on save for Go files. Preserve the original file when formatting fails; show formatting and write errors in the UI. Test search wraparound and save failure paths.
- [ ] 6. Add Go syntax highlighting as styled spans between the UTF-8 buffer and renderer. Keep highlighting out of editor state, and verify colors and cursor placement on SSH/tmux-compatible terminals.

## Build loop

- [ ] 7. Detect the user's `go` executable and version, then add cancellable background jobs for `go build ./...`, `go test ./...`, and `go run .` in the project root. Stream stdout/stderr into an output pane, show job state, and prevent stale or cancelled job output from replacing a newer run.
- [ ] 8. Parse Go compiler and test output into one diagnostic model with path, line, column, severity, source, and message. Retain unmatched lines as plain output. Test relative and absolute paths, malformed lines, and diagnostics without a column.
- [ ] 9. Make diagnostics selectable and jump to the correct file and source position. Define behavior for missing files and out-of-range positions, and keep the output pane useful after navigation.

## Usability and acceptance

- [ ] 10. Publish all MVP shortcuts in an in-app help surface and README. Verify conventional keys and function keys in at least one Linux SSH session and one tmux session, including terminal resize and UTF-8 text.
- [ ] 11. Run the full [MVP acceptance test](docs/MVP.md): introduce a compile error, save/format, build, inspect the diagnostic, jump to the location, fix, rebuild, and run without leaving RapidGo. Add automated integration coverage where practical and document any remaining terminal-specific limitations before v0.1.

## Optional usability follow-ups (not v0.1 blockers)

- [ ] Add Help → Keyboard Test as a RapidGo troubleshooting aid, not as a standard Turbo Pascal or Borland C++ feature. Show a 12 × 4 matrix for F1–F12 with no modifier, Shift, Ctrl, and Alt; leave multi-modifier combinations out initially.
  - For each prompted combination, distinguish received as expected, received as a different key, and not received before a timeout. Show the key and modifiers RapidGo interpreted, plus the raw terminal sequence only if the input layer can provide it.
  - While the test is open, handle its function-key input before normal IDE shortcuts so F1/F6/F10 can be tested; Esc closes the screen.
  - Do not expect to detect Fn separately: the keyboard/OS usually handles it before the terminal sends a key. Explain that the screen diagnoses terminal/OS/tmux mappings but does not try to repair them.

## After v0.1

- [ ] 12. v0.2: add gopls through an adapter for completion, diagnostics, definition, references, and hover.
- [ ] 13. v0.3: add a coherent Delve debugger model with breakpoints, stepping, stack frames, goroutines, locals, and watches.
