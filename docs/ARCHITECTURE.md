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

The project tree in `internal/project` loads only opened directories and marks a directory as a Go module when its `go.mod` is encountered. `internal/ui` runs directory scans and bounded UTF-8 file reads on background workers, then applies results on the UI loop. The UI owns focus across the tree, editor, and output panes and indicates it with a double-line border; inactive panes use single-line borders. The focused output pane takes a larger share of the work area, and a narrow terminal that can show only one work pane keeps displaying the tree or editor the output pane was reached from. Transient messages have their own row above the status bar so job output does not displace them. The editor buffer owns text and editing state; the UI maps keyboard input to buffer operations and renders its grapheme positions as terminal cells.

## State and concurrency

The UI loop owns visible application state. Background jobs send typed events to it and never mutate UI state directly. Each job has a context, stable identity, lifecycle state, output stream, and zero or more diagnostics. Starting a replacement job cancels the prior job of the same kind after making the transition visible.

`internal/jobs` keeps one job slot per kind. Starting a kind cancels its previous job, waits for that process to exit, and only then starts the replacement, so the output of two runs of one kind never interleaves. Every event carries the job identity; the UI keeps one view per kind and discards events whose identity does not match the run it is showing, which is what prevents a cancelled or superseded job from replacing a newer result. The UI drains the event channel in bursts and redraws once per burst rather than once per output line, and it bounds retained output per job. Each kind keeps its own scrollback and viewport; a view follows the end of its output until the user scrolls away and resumes following when the viewport returns to the last line, including when a resize brings it there rather than a keystroke. Focus never moves to the output pane on a terminal too short to render it.

Job output is split into lines as it arrives, with standard output and standard error reported separately and a per-line size cap. Cancellation interrupts the job's process group, so the program a `go run .` job started stops with the job; `os/exec`'s wait delay then kills the process and closes its pipes if it ignores the interrupt, and any survivor of the group is killed once the job is reaped. Process groups are a Unix mechanism, so on other platforms cancellation reaches only the go executable itself.

The editor buffer stores UTF-8 text while cursor and selection operations use well-defined text positions rather than terminal cell offsets. Rendering is responsible for converting text positions into terminal cells, including wide and combining characters.

Go highlighting uses the standard-library scanner in `internal/highlight` to produce lexical UTF-8 byte spans without terminal styles. The UI caches those spans by buffer identity and revision, maps them onto visible grapheme clusters, and lets selection styling take precedence. Other files keep the base editor color.

Editor positions use zero-based lines and grapheme-cluster columns. The buffer retains UTF-8 byte offsets internally for edits and undo/redo, while the terminal renderer maps grapheme positions to visual cells. Literal search and save checkpoints live in the buffer; formatting and disk I/O remain separate, on background workers. A successful formatter result becomes an undoable buffer edit only if no newer edits superseded its save snapshot.

The buffer treats LF as the logical line break. Loading CRLF removes only its final CR, preserving any preceding bare CRs; inserted CRLF text is handled the same way. A bare CR remains editable even when an edit places it next to LF. Serialization adds an extra CR for such a break so save/reload preserves the bare character. For mixed-line-ending input, the first unambiguous newline determines the default output style; if every break is ambiguous, CRLF is the fallback. Go files use the external `gofmt` output on save, which may normalize line endings.

## External processes

Go is external and user-managed. RapidGo resolves `go` on `PATH` once per session, off the UI loop, and reports its version in the help surface; a missing toolchain is reported when detection finishes and again when a command is requested. RapidGo does not manage Go installations, but it also does not suppress Go's own toolchain selection: under the default `GOTOOLCHAIN=auto`, the executable RapidGo launches may download and hand off to a newer toolchain named by `go.mod`, so the detected version is the executable invoked rather than a guarantee about the compiler used. Commands run in the selected project root. MVP commands are:

- `gofmt` on save
- `go build ./...`
- `go test ./...`
- `go run <main package>`
- `go list -e -json ./...` to find the runnable packages for `go run`

`go run` needs exactly one main package, and a Go project's executable is usually under `cmd/` rather than in the root, so the run target is resolved rather than assumed. `internal/jobs` reports the module's runnable packages from `go list`; the UI caches that listing and drops it on every successful save, because RapidGo does not watch the filesystem and a save can change which packages are runnable. The cache carries a generation, so a listing that was already running when the project changed is discarded rather than adopted, and re-requested when a run is waiting on it. This is the same identity check the job events use. `internal/ui` owns the policy, preferring the main package of the open file, then a sole main package, then the target chosen earlier in the session, and otherwise asking. Resolution uses the package clause Go itself reports instead of inspecting source, and runs the package rather than a file so build tags apply. No rule keys off directory names: a runnable package is runnable wherever it lives, and the distinction that matters is between a target the user selected or is editing and one RapidGo picked because it was unambiguous.

Command arguments are constructed directly rather than through a shell. Output that matches a known Go diagnostic is structured; all other output remains visible verbatim.

`internal/diagnostic` parses output lines into one model of path, line, column, severity, source, package, and message. It is stateful by necessity: Go names a package once in a `# package` header above the errors in it, and marks a failing test once above the lines explaining the failure. A path is kept exactly as the tool wrote it, which may be relative to the project root, relative to a package directory for test output, or absolute; resolving it belongs to whatever navigates to it. A located line is recognized only when its path ends in `.go`, which keeps timestamps out. That is not enough for `go run`, whose output also carries the program's own writing and where `log.Lshortfile` prints `main.go:7: msg`. A parser told its output may be a program's records diagnostics only once a `# package` header has appeared, which means compilation failed and the program never started, so the header holds for the rest of the run. A program that prints such a header itself would be misread, which combined `go run` output cannot distinguish. A subtest's result governs only the lines indented inside it, so a parent's later output is not graded by a subtest that has ended. Go's plain test output cannot distinguish a `t.Log` from a `t.Errorf`, so every located line inside a failing test counts as evidence of that failure; `go test -json` would remove the ambiguity. Process cancellation must terminate child processes and prevent stale events from replacing newer results.

## Deferred integrations

gopls belongs to v0.2 and communicates through an adapter rather than entering editor state directly. Delve belongs to v0.3 and adds an explicit debugger domain model. Neither is an MVP dependency.
