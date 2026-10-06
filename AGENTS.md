# Engineering standards for RapidGo

RapidGo is a keyboard-first terminal IDE for Go. Keep the core editing and job model independent from the terminal UI so it can be tested without a terminal and reused by future frontends.

## Project boundaries

- The roadmap in `README.md` defines the current milestone. Do not pull a Git UI, an embedded shell, plugins, or AI into a milestone unless the roadmap adds them.
- Users own their Go installation. RapidGo may detect and invoke Go, but it must not install or switch Go toolchains.
- RapidGo may eventually manage IDE-specific tools beneath a platform-appropriate user data directory.
- Treat SSH, tmux, terminal resize, narrow terminals, and UTF-8 text as normal environments rather than edge cases.

## Design expectations

- Keep document buffers, selections, undo/redo, commands, diagnostics, jobs, and language-server sessions independent from terminal widgets and rendering.
- Pass dependencies such as filesystems, clocks, and command runners through narrow interfaces where doing so makes behavior deterministic and testable.
- Make background work cancellable with `context.Context`; never block the UI event loop on builds, tests, formatting, or filesystem walks.
- Normalize build, test, run, and gopls problems into one diagnostic model with file, line, column, severity, source, and message fields.
- Preserve unknown output as plain job output even when it cannot be parsed as a diagnostic.
- Prefer conventional, discoverable shortcuts. List new keybindings in the in-app help (`helpEntries` in `internal/ui/render_help.go`) and in [`docs/GUIDE.md`](docs/GUIDE.md).
- Put user-visible text in `internal/i18n/packs/en_US.json` and display it through `i18n.Text`, `i18n.Format`, or `i18n.Errorf`, keeping paths and other runtime values in placeholders. See [`docs/I18N.md`](docs/I18N.md).

## Go quality

- Write idiomatic Go; run `gofumpt` and `goimports` on changed Go files.
- Handle errors explicitly at the appropriate boundary and return messages that identify what failed.
- Avoid goroutine, process, file descriptor, and channel leaks and unnecessary allocations. Test cancellation paths.
- Keep packages cohesive and public APIs focused; do not expose internal details or couple core state to a specific TUI library.
- Keep declaration comments to one line. Explain non-obvious intent or reasoning next to the relevant code without restating it; reconsider comments longer than three lines and put broader context in `docs/ARCHITECTURE.md`.
- Prefer non-test Go files under 300 lines and keep them at or below 500 lines. `make lint` enforces the 500-line limit through revive; test files are exempt and may be longer when coverage warrants it. Split larger files into cohesive units.
- Run `make check` before merging.

## Testing and TDD

- Pair each source file that contains executable behavior with a corresponding `foo_test.go`; use matching build tags for platform-specific code. Test declarations and constants through the behavior that uses them.
- Cover typical behavior, errors, cancellation, and UTF-8 or terminal-size boundaries where relevant.
- Use table-driven tests for parsers and core state transitions.
- For new behavior and bug fixes, write a failing test first and observe the failure before changing the implementation. Coverage work for existing behavior does not need an artificial failure.
- If golden tests are introduced, keep approved fixtures under `testdata/golden/`, generate candidates under `testdata/gen/` with Go where practical, and provide a repeatable update script.

## Contributions

- For every feature or bug fix, create a dedicated branch from `main` before changing code, run the quality gate, commit the changes, push the branch, and open a pull request against `main`. Do not stop at uncommitted local changes.
- Stage the intended changes before running `make check` so its final diff check detects formatting or dependency changes made by the gate. Review and stage any such changes, then rerun the gate before committing.
- Use Conventional Commits.
- Keep changes small and aligned with the current milestone. Track follow-ups and deferred work in GitHub issues rather than files in the repository.
- Keep `README.md` short: pitch, quick start, screenshots, install, and roadmap. Describe feature behavior in `docs/GUIDE.md` and design in `docs/ARCHITECTURE.md`. Keep README text captures within 90 columns so GitHub shows them without scrolling.
- Do not commit generated binaries, coverage output, local settings, or downloaded fixtures.
- Do not add assistant attribution, generated-by notes, or co-author metadata to commits or repository files.
