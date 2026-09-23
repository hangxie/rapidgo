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
- `internal/project`: root discovery, safe filesystem traversal, and project-tree state.
- `internal/highlight`: transforms text and language metadata into styled spans without rendering them.
- `internal/jobs`: cancellable asynchronous build, test, run, and format processes plus output streaming.
- `internal/diagnostic`: common diagnostic representation and parsers for Go command output.
- `internal/ui`: terminal event mapping, layout, rendering, focus, dialogs, and shortcut help.

Packages should be introduced as their behavior is implemented; this list describes boundaries, not required empty directories.

The v0.1 shell uses `tcell/v2` inside `internal/ui` for screen cells, keyboard events, and resize events. It uses a simulated screen in tests. The editor buffer and other domain packages must not import `tcell`.

## State and concurrency

The UI loop owns visible application state. Background jobs send typed events to it and never mutate UI state directly. Each job has a context, stable identity, lifecycle state, output stream, and zero or more diagnostics. Starting a replacement job cancels the prior job of the same kind after making the transition visible.

The editor buffer stores UTF-8 text while cursor and selection operations use well-defined text positions rather than terminal cell offsets. Rendering is responsible for converting text positions into terminal cells, including wide and combining characters.

## External processes

Go is external and user-managed. RapidGo detects the executable and reports its version, then invokes commands in the selected project root. MVP commands are:

- `gofmt` on save
- `go build ./...`
- `go test ./...`
- `go run .`

Command arguments are constructed directly rather than through a shell. Output that matches a known Go diagnostic is structured; all other output remains visible verbatim. Process cancellation must terminate child processes and prevent stale events from replacing newer results.

## Deferred integrations

gopls belongs to v0.2 and communicates through an adapter rather than entering editor state directly. Delve belongs to v0.3 and adds an explicit debugger domain model. Neither is an MVP dependency.
