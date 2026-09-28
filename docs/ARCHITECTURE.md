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
- `internal/ui`: terminal event mapping, layout, rendering, focus, dialogs, and shortcut help.

Packages should be introduced as their behavior is implemented; this list describes boundaries, not required empty directories.

The v0.1 shell uses `tcell/v2` inside `internal/ui` for screen cells, keyboard events, and resize events. It uses a simulated screen in tests. The editor buffer and other domain packages must not import `tcell`.

The project tree in `internal/project` loads directories only when opened and marks them as Go modules when it finds `go.mod`. It does not follow directory symlinks. A 64-level limit also bounds parent walks and visible-tree traversal if a cyclic tree reaches the model. Directories at that depth remain visible but cannot be expanded.

`internal/ui` runs directory scans and bounded UTF-8 file reads on background workers, then applies the results on the UI loop. The UI owns focus across the tree, editor, and output panes. It marks the focused pane with a double-line border and gives the output pane more space when focused. On a narrow terminal that can show only one work pane, it keeps the tree or editor visible when focus moves to output. Transient messages have a row above the status bar so job output does not displace them.

The editor buffer owns text and editing state. The UI maps keys to buffer operations and renders grapheme positions as terminal cells.

## State and concurrency

The UI loop owns visible application state. Background jobs send typed events to it and never mutate UI state directly. Each job has a context, stable identity, lifecycle state, output stream, and zero or more diagnostics. Starting a replacement job cancels the prior job of the same kind after making the transition visible.

`internal/jobs` keeps one job slot per kind. Starting a kind cancels its previous job, waits for that process to exit, and only then starts the replacement, so the output of two runs of one kind never interleaves. Every event carries the job identity; the UI keeps one view per kind and discards events whose identity does not match the run it is showing, which is what prevents a cancelled or superseded job from replacing a newer result. The UI drains the event channel in bursts and redraws once per burst rather than once per output line, and it bounds retained output per job. Each kind keeps its own scrollback and viewport; a view follows the end of its output until the user scrolls away and resumes following when the viewport returns to the last line, including when a resize brings it there rather than a keystroke. Focus never moves to the output pane on a terminal too short to render it.

Job output is split into lines as it arrives, with standard output and standard error reported separately and a per-line size cap. Cancellation interrupts the job's process group, so the program a `go run .` job started stops with the job; `os/exec`'s wait delay then kills the process and closes its pipes if it ignores the interrupt, and any survivor of the group is killed once the job is reaped. Process groups are a Unix mechanism, so on other platforms cancellation reaches only the go executable itself.

The editor buffer stores UTF-8 text while cursor and selection operations use well-defined text positions rather than terminal cell offsets. Rendering is responsible for converting text positions into terminal cells, including wide and combining characters.

Go highlighting uses the standard-library scanner in `internal/highlight` to produce lexical UTF-8 byte spans without terminal styles. The UI caches those spans by buffer identity and revision, maps them onto visible grapheme clusters, and lets selection styling take precedence. Other files keep the base editor color.

Editor positions use zero-based lines and grapheme-cluster columns. The buffer retains UTF-8 byte offsets internally for edits and undo/redo, while the terminal renderer maps grapheme positions to visual cells. Literal search and save checkpoints live in the buffer; formatting and disk I/O remain separate, on background workers. A successful formatter result becomes an undoable buffer edit only if no newer edits superseded its save snapshot.

The buffer treats LF as the logical line break. Loading CRLF removes only its final CR, preserving any preceding bare CRs; inserted CRLF text is handled the same way. A bare CR remains editable even when an edit places it next to LF. Serialization adds an extra CR for such a break so save/reload preserves the bare character. For mixed-line-ending input, the first unambiguous newline determines the default output style; if every break is ambiguous, CRLF is the fallback. Go files use the external `gofmt` output on save, which may normalize line endings.

## External processes

Go is external and user-managed. RapidGo resolves `go` on `PATH` once per session, off the UI loop, and reports its version and full executable path in Help → Environment; a missing toolchain is reported when detection finishes and again when a command is requested. RapidGo does not manage Go installations, but it also does not suppress Go's own toolchain selection: under the default `GOTOOLCHAIN=auto`, the executable RapidGo launches may download and hand off to a newer toolchain named by `go.mod`, so the detected version is the executable invoked rather than a guarantee about the compiler used. Commands run in the selected project root. MVP commands are:

- `gofmt` on save
- `go build ./...`
- `go test -json ./...`
- `go run <main package>`
- `go run <current entry.go> <active helpers.go>` for Run Current Entry, or `go run <package>` when assembly requires it
- `go list -e -json ./...` to find the runnable packages for `go run`

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

Run Current Entry lists and runs from the entry file's directory. It uses GOPATH mode only when that directory has no module or workspace in its ancestry. Explicit `GO111MODULE` and external `GOWORK` settings take precedence.

### Multiple main files

If one package contains several files with top-level `main()` functions, running the whole package produces a duplicate definition error. This can happen inside a module, such as in an `examples/` directory, or in a standalone directory. Run stays package-oriented and leaves that error visible.

Run Current Entry first requires the open buffer to be saved. It checks the file on disk for package `main` and a top-level `main()` function. Then it asks `go list` for the directory's active `GoFiles` and `CgoFiles`; the entry must be among them. The run list contains the entry and active, same-package, non-test siblings without another `main()`.

When assembly or active cgo/SWIG companion files require a package run, RapidGo runs the package if only one main is active. Otherwise it reports a clear error. Go's current `GOOS`, `GOARCH`, and build tags determine which files are active. An excluded entry reports a build-constraint error; users can set tags through `GOFLAGS`, for example `GOFLAGS=-tags=example`. Go package load failures retain Go's message. Build and Test keep their project-wide commands.

Command arguments are constructed directly rather than through a shell. Output that matches a known Go diagnostic is structured; all other output remains visible verbatim. A separate terminal run action suspends tcell input and rendering, runs the resolved command with the terminal streams attached, and restores RapidGo after the child exits. It suspends the screen rather than finishing it because tcell finishes a screen only once, and spending that on a handoff would leave the shell in raw mode on the alternate screen when RapidGo later quits. It requires other Go jobs to be stopped so their output cannot block while the UI is suspended. The attached child owns terminal interaction; its output is not parsed into the integrated pane.

`internal/diagnostic` parses compiler and test output into path, position, severity, source, package, and message.

- **Context:** The parser tracks package headers and nested test failures across lines. JSON events name their package directly; a later verdict can supply it for plain output.
- **Test severity:** Go 1.27 marks logs and errors separately. Go 1.26 lacks that mark, so a failure event grades located output from the failed test as errors.
- **Run output:** Diagnostic parsing starts only after a compiler `# package` header, avoiding confusion with program logs. Unknown output stays visible; compiler `have` and `want` lines also extend the preceding diagnostic.
- **Source jumps:** The parser preserves reported paths. The UI keeps each diagnostic on its output line, resolves package-relative paths with `go list`, and converts byte columns to editor grapheme positions when opening a file.

## Deferred integrations

gopls belongs to v0.2 and communicates through an adapter rather than entering editor state directly. Delve belongs to v0.3 and adds an explicit debugger domain model. Neither is an MVP dependency.
