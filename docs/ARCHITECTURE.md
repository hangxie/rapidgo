# Architecture

RapidGo separates domain state from terminal rendering. The editor, project tree, diagnostics, and job lifecycle must be usable in tests without initializing a terminal.

## Data flow

```text
filesystem ──> project model ───────────────┐
                                            │
UTF-8 file ─> editor buffer ─> highlighter ├─> TUI renderer/input
                                            │
Go command ─> job output ────> diagnostics ┘
                                 │
                                 └─> source location
```

## Intended package boundaries

- `cmd/rapidgo`: process entry point and version wiring only.
- `internal/editor`: text buffers, cursor and selection state, edits, undo/redo, search, and save state. No terminal types.
- `internal/project`: root discovery, safe filesystem traversal, project-tree state, and temporary-file replacement on save.
- `internal/highlight`: transforms text and language metadata into styled spans without rendering them.
- `internal/jobs`: cancellable asynchronous build, test, and run processes plus output streaming.
- `internal/diagnostic`: common diagnostic representation and parsers for Go command output.
- `internal/gopls`: LSP framing and a cancellable gopls process session, with versioned document sync, diagnostic notifications, hover, completion, definition, and references requests. The UI starts it in a background worker when a Go file opens. The UI keeps per-file diagnostic reports for the project, clears them on empty notifications, and converts UTF-16 columns when an editor buffer is available.
- `internal/ui`: terminal event mapping, layout, rendering, focus, dialogs, and shortcut help.

These boundaries describe implemented behavior, not empty packages to create in advance.

### Project loading and terminal UI

The project tree loads directories when they are opened. It marks a directory as a Go module when it encounters `go.mod` and refuses symlink directories. A separate 64-level cap bounds the parent walk during expansion and recursive visible-tree traversal even if a cyclic tree reaches the model. This is a defensive limit; directories at depth 64 remain visible but cannot be expanded. Directory scans and bounded UTF-8 file reads run on background workers; the UI applies their results on its event loop.

The v0.1 shell uses `tcell/v2` for screen cells, keyboard events, and resize events. Tests use a simulated screen. Domain packages such as `internal/editor` do not import `tcell`.

| Surface | UI behavior |
| --- | --- |
| Tree, editor, and bottom panes | The UI owns focus. A double-line border marks the focused pane; inactive panes have single-line borders. The bottom pane selects Output or Errors without changing its layout. |
| Focused output pane | It receives a larger share of the work area. |
| Narrow terminal | If only one work pane fits, the output pane preserves the tree or editor pane from which it was reached. |
| Messages and status | Transient messages occupy a row above the status bar, away from job output. |

The editor buffer owns text and editing state. The UI maps keys to buffer operations and renders grapheme positions as terminal cells.

## State and concurrency

### Job lifecycle

The UI loop owns visible state. Background jobs send typed events and never mutate that state directly. Each job has a context, stable identity, lifecycle state, output stream, and zero or more diagnostics.

The bottom pane keeps a mode independent of keyboard focus. Starting a job selects Output; a failed job with a located error selects Errors but leaves focus unchanged. gopls reports update the Errors data without selecting that mode. Explicit references and multiple-definition requests select Locations and focus the list. Errors reads located problems from the selected job and the current per-file gopls reports; Output keeps every job line, including text that did not parse as a diagnostic.

`internal/jobs` keeps one slot per command kind. The job lifecycle follows these rules:

- A new request makes the replacement visible, cancels the previous job of the same kind, and waits for it to exit before starting the next process. Their output cannot interleave.
- Every event carries its job identity. The UI keeps one view per kind and discards events from a cancelled or superseded run.
- The UI drains events in bursts and redraws once per burst. It also limits the output retained for each job.
- Each kind has its own scrollback and viewport. A view follows new output until the user scrolls away. It resumes following when the viewport returns to the last line, including after a resize.

Focus never moves to the output pane if the terminal is too short to show it.

Job output arrives as bounded lines. Standard output and standard error remain separate. Cancellation interrupts the job's process group, including a program started by `go run`. If it ignores the interrupt, `os/exec`'s wait delay kills the process and closes its pipes; any remaining group member is killed after the job is reaped. On platforms without Unix process groups, cancellation reaches only the Go executable.

### Text and rendering

| Concern | Representation or owner |
| --- | --- |
| Text and edits | The editor buffer stores UTF-8 text and byte offsets for edits and undo/redo. |
| Cursor and selection | Positions use zero-based lines and grapheme-cluster columns. |
| Terminal display | The renderer maps grapheme positions to cells, including wide and combining characters. |
| Go highlighting | `internal/highlight` scans lexical UTF-8 byte spans without terminal styles. The UI caches spans by buffer identity and revision, maps them to visible graphemes, and gives selection styling priority. Other files keep the base editor color. |

Literal search and save checkpoints live in the buffer. Formatting and disk I/O run on background workers. A formatter result becomes an undoable edit only if no newer edits superseded its save snapshot.

The buffer uses LF as its logical line break. When loading or inserting CRLF text, it removes only the final CR and preserves any preceding bare CR. A bare CR remains editable even next to LF; serialization adds an extra CR to preserve that character across save and reload.

For mixed line endings, the first unambiguous newline sets the default output style. CRLF is the fallback when every break is ambiguous. Saving a Go file uses external `gofmt` output, which may normalize line endings.

## External processes

### Go toolchain

The user owns the Go installation. RapidGo resolves `go` on `PATH` once per session, away from the UI loop. Help → Environment shows the executable path and detected version. A missing toolchain is reported when detection finishes and again when a command is requested.

RapidGo leaves Go's toolchain selection in place. With the default `GOTOOLCHAIN=auto`, Go may download and use a newer toolchain required by `go.mod`. The version shown in Help is the executable RapidGo launched, which might differ from the compiler that runs.

Project-wide commands run from the selected project root. Run Current File uses the entry file's directory.

| Action | Default command |
| --- | --- |
| Format on save | `gofmt` |
| Build | `go build ./...` |
| Test | `go test -json ./...` |
| Run | `go run <main package>` |
| Run Current File | `go run <entry.go> <active helpers.go>`; `go run <package>` when assembly requires it |
| Discover runnable targets | `go list -e -json ./...` |

### Run targets

`go run` needs one runnable target. RapidGo uses `go list -e -json ./...` to find packages whose reported package name is `main`, including executables under `cmd/` or any other directory. Package names and Go's selected source files determine the targets; directory names do not.

When the first program argument to a file-list run ends in `.go`, `go run` would treat it as another source file. RapidGo instead builds the selected files into a temporary executable under the entry directory, then launches it with the program arguments. If the entry directory is not writable, it uses the OS temporary directory. If the binary cannot start, it reports a possible `noexec` mount.

The UI chooses a target in this order:

1. The runnable target containing the open entry file.
2. The only runnable target, if there is one.
3. The target chosen earlier in this session.
4. A target the user picks from a chooser.

The UI caches the package listing until a successful save or explicit rescan. A save can change which packages are runnable, so it invalidates the cache. Each listing carries a generation: if a save happens while `go list` runs, the UI discards that result and requests a fresh listing when an action is waiting for one.

### Standalone directories

Project-wide commands use GOPATH mode when the root has no `go.mod` or `go.work` in its ancestry or subtree. Concurrent commands share the subtree search. The cached result is invalidated when a visited directory changes and refreshed after two seconds to account for coarse filesystem timestamps.

Run Current File lists and runs from the entry file's directory. It uses GOPATH mode only when that directory has no module or workspace in its ancestry. Explicit `GO111MODULE` and external `GOWORK` settings take precedence.

### Multiple main files

If one package contains several files with top-level `main()` functions, running the whole package produces a duplicate definition error. This can happen inside a module, such as in an `examples/` directory, or in a standalone directory. Run stays package-oriented and leaves that error visible.

Run Current File first requires the open buffer to be saved. It checks the file on disk for package `main` and a top-level `main()` function. Then it asks `go list` for the directory's active `GoFiles` and `CgoFiles`; the entry must be among them. The run list contains the entry and active, same-package, non-test siblings without another `main()`.

When assembly or active cgo/SWIG companion files require a package run, RapidGo runs the package if only one main is active. Otherwise it reports a clear error. Go's current `GOOS`, `GOARCH`, and build tags determine which files are active. An excluded entry reports a build-constraint error; users can set tags through `GOFLAGS`, for example `GOFLAGS=-tags=example`. Go package load failures retain Go's message. Build and Test keep their project-wide commands.

### Command output and terminal handoff

RapidGo constructs command arguments directly, without a shell. It records recognized Go diagnostics as structured data and leaves unknown output visible as plain text.

Run in Terminal suspends tcell input and rendering, then runs the resolved command with the terminal streams attached. The child owns terminal interaction; its output does not enter the integrated pane or diagnostic parser. Other Go jobs must be stopped first so their output cannot block while the UI is suspended.

When the child exits, RapidGo restores the screen. It suspends rather than finishes the tcell screen because tcell permits only one finish. Finishing during handoff would leave the shell in raw mode on the alternate screen when RapidGo later quits.

### Diagnostics

`internal/diagnostic` parses compiler and test output into path, position, severity, source, package, and message.

- **Context:** The parser tracks package headers and nested test failures across lines. JSON events name their package directly; a later verdict can supply it for plain output.
- **Test severity:** Go 1.27 marks logs and errors separately. Go 1.26 lacks that mark, so a failure event grades located output from the failed test as errors.
- **Run output:** Diagnostic parsing starts only after a compiler `# package` header, avoiding confusion with program logs. Unknown output stays visible; compiler `have` and `want` lines also extend the preceding diagnostic.
- **Source jumps:** The parser preserves reported paths. The UI keeps each diagnostic on its output line, resolves package-relative paths with `go list`, and converts byte columns to editor grapheme positions when opening a file.

## Deferred integrations

gopls belongs to v0.2 and communicates through an adapter outside editor state. Completion and source navigation are explicit, time-bounded requests on the language worker. The UI accepts a response only for the same document text and caret; its suggestion and location lists are terminal-specific, while UTF-16 positions and completion edit validation stay outside rendering. Plain-text completion and additional import edits are applied as one undoable change. Delve belongs to v0.3 and adds an explicit debugger domain model. Neither is an MVP dependency.
