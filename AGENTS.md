# Engineering standards for RapidGo

RapidGo is a keyboard-first terminal IDE for Go. Keep the core editing and job model independent from the terminal UI so it can be tested without a terminal and reused by future frontends.

## Project boundaries

- The v0.1 scope is documented in `docs/MVP.md`. Do not pull gopls, Delve, Git UI, an embedded shell, plugins, or AI into the MVP.
- Users own their Go installation. RapidGo may detect and invoke Go, but it must not install or switch Go toolchains.
- RapidGo may eventually manage IDE-specific tools beneath a platform-appropriate user data directory.
- Treat SSH, tmux, terminal resize, narrow terminals, and UTF-8 text as normal environments rather than edge cases.

## Design expectations

- Keep document buffers, selections, undo/redo, commands, diagnostics, and jobs independent from terminal widgets and rendering.
- Pass dependencies such as filesystems, clocks, and command runners through narrow interfaces where doing so makes behavior deterministic and testable.
- Make background work cancellable with `context.Context`; never block the UI event loop on builds, tests, formatting, or filesystem walks.
- Normalize build, test, and run output into one diagnostic model with file, line, column, severity, source, and message fields.
- Preserve unknown output as plain job output even when it cannot be parsed as a diagnostic.
- Prefer conventional, discoverable shortcuts. New keybindings must be represented in the in-app help surface and user documentation.

## Go quality

- Write idiomatic Go and handle errors explicitly at the appropriate boundary.
- Add table-driven tests for parsers and state transitions, including malformed input and UTF-8 edge cases.
- Avoid goroutine, process, file descriptor, and channel leaks. Test cancellation paths.
- Keep packages cohesive and avoid coupling core state to a specific TUI library.
- Comment a package, type, function, field, or any other declaration in one line. Needing more is a sign the declaration is unclear: rename it, split it, or narrow what it does until one line covers it.
- Keep every other comment to two lines at most. Context that cannot live in code, such as how an external tool behaves, belongs in `docs/ARCHITECTURE.md`.
- Run `make check` before merging.

## Contributions

- Use Conventional Commits.
- Keep changes small and aligned with the current milestone.
- Do not commit generated binaries, coverage output, local settings, or downloaded fixtures.
- Do not add assistant attribution, generated-by notes, or co-author metadata to commits or repository files.

