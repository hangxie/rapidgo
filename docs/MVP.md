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
- Run `go build ./...`, `go test ./...`, and `go run` on a resolved main package asynchronously.
- Show command output in an integrated pane.
- Parse Go compiler and test diagnostics containing file, line, and column.
- Select a diagnostic and jump to the exact source location.
- Expose keyboard shortcuts and function keys through a discoverable help surface.
- Behave correctly over SSH and tmux, on terminal resize, and with UTF-8 source.

## Tool ownership

The user owns the installed Go toolchain. RapidGo detects and invokes it but does not install, upgrade, or switch Go versions itself. Go's own toolchain selection is left alone, so the `go` executable RapidGo launches may still download and hand off to a newer toolchain named by `go.mod` under the default `GOTOOLCHAIN=auto`. IDE-specific tools may later be installed beneath a platform-appropriate data directory such as `~/.local/share/rapidgo/bin` on Linux.

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

- [x] Describe the project tree as Borland C++-style, not a Turbo Pascal 7 feature; TP7 used a primary file and per-project configuration instead.
- [x] Keep SIGINT/SIGTERM independent of the terminal event queue so a full queue cannot swallow shutdown.
- [x] Keep help and quit hints visible in the status bar even when a narrow terminal shows a long project name.
- [x] Use an explicit VGA-inspired palette, including blue/yellow work areas, gray menu and status surfaces, and red shortcut text.
- [x] Make the menu bar keyboard-operable while keeping shortcut help discoverable.
- [x] Switch focused panes with F6 or Ctrl+F6, leaving Tab available for editing as in the Borland C++ IDE.
- [x] Place project context in a window title and use framed dropdown menus instead of a separate application header and one-line menus.
- [x] With keyboard-operable tree/preview focus, use a double-line border for the active pane and single-line borders for inactive panes.
- [x] Give Go commands their own Build menu and function keys. Borland had no test command, so `Ctrl+T` (test) and `Ctrl+K` (stop) are RapidGo additions, and the WordStar-style editor command set those keys belong to is not implemented.
- [x] Cancel jobs through the process group so the program started by `go run .` stops with its job instead of outliving RapidGo.
- [x] Resolve the `go run` target instead of assuming `go run .`. A Go project's root is usually a library with the executable under `cmd/`, so RapidGo discovers main packages with `go list` and prefers the one being edited. Classify runnable packages by the package clause Go reports, never by directory names such as `examples` or `demo`, which are project-specific and unreliable.
- [x] Document that RapidGo does not manage Go installations but also does not override `GOTOOLCHAIN`, so the detected version is the executable invoked rather than a promise about the compiler used.
- [x] Make the stop shortcut cancel every running command. The output pane shows one kind at a time, so cancelling only the visible job could leave an invisible process running.
- [x] Give transient messages their own row above the status bar. Sharing the output pane meant job output displaced them once a Go command had run.
- [x] Make the output pane focusable and scrollable, and let the focused pane grow. A quarter of the work area is too little to read a test failure, and output that only tails cannot be reviewed.
- [x] Treat Linux, macOS, and Windows as intended runtime platforms, with BSD and other server Unix systems best effort. Android and iOS may be SSH clients but are not RapidGo runtime targets.
