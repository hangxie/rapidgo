# v0.1 MVP scope

## Product goal

RapidGo is a lightweight, keyboard-first TUI IDE for Go, optimized for remote Linux development over SSH. It should feel like a small integrated environment, not a modal editor configuration.

## Required capabilities

- Open a Go module or directory with `rapidgo .`.
- Browse a project/file tree and open files.
- Edit text with conventional non-modal cursor movement and selection.
- Display Go syntax highlighting and terminal colors.
- Save files and search within the current file.
- Run `gofmt`, normally on save.
- Run `go build ./...`, `go test ./...`, and `go run .` asynchronously.
- Show command output in an integrated pane.
- Parse Go compiler and test diagnostics containing file, line, and column.
- Select a diagnostic and jump to the exact source location.
- Expose keyboard shortcuts and function keys through a keyboard-operable menu bar and discoverable help surface.
- Behave correctly over SSH and tmux, on terminal resize, and with UTF-8 source.

## Tool ownership

The user owns the installed Go toolchain. RapidGo detects and invokes it but does not install, upgrade, or switch Go versions. IDE-specific tools may later be installed beneath a platform-appropriate data directory such as `~/.local/share/rapidgo/bin` on Linux.

## Explicitly out of scope

- gopls/LSP completion, hover, definitions, references, and LSP diagnostics (v0.2)
- Delve and interactive debugging (v0.3)
- Git UI
- Embedded shell or terminal emulator
- Plugins
- AI features
- Browser or GUI frontend
- Remote filesystem protocol
- Go installation or version management

Delve is deferred because a useful debugger needs coherent breakpoints, execution state, stepping, stack frames, goroutines, locals, and watches. A partial debugger would prevent the MVP from testing its core editing loop.

## Acceptance test

On a machine reached over SSH:

1. Run `rapidgo .` inside a Go project.
2. Open a Go file and introduce a compile error.
3. Save and format the file.
4. Build the project and see the diagnostic.
5. Select the diagnostic and jump to its exact source location.
6. Fix the error and rebuild successfully.
7. Run the program without leaving RapidGo.

## Things found during implementation

- [x] Keep SIGINT/SIGTERM independent of the terminal event queue so a full queue cannot swallow shutdown.
- [x] Keep help and quit hints visible in the status bar even when a narrow terminal shows a long project name.
- [x] Use an explicit VGA-inspired palette, including blue/yellow work areas, gray menu and status surfaces, and red shortcut text.
- [x] Place project context in a window title and use framed dropdown menus instead of a separate application header and one-line menus.
- [ ] When pane focus exists, use a double-line border for the active pane and single-line borders for inactive panes; do not add visual-only focus state beforehand.
