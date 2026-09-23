# RapidGo

RapidGo is a lightweight, keyboard-first terminal IDE for Go. It is designed for remote Linux machines where the whole development loop happens over SSH:

```sh
ssh devbox
cd project
rapidgo .
```

The interface is inspired by integrated, discoverable environments such as Turbo Pascal: conventional non-modal editing, visible shortcuts, a project tree, and build diagnostics in one terminal application. It is intentionally not a Vim or Neovim configuration.

> [!IMPORTANT]
> RapidGo is pre-alpha. `rapidgo .` opens a terminal shell with a menu bar and project, editor, and output panes. Browsing, editing, and Go jobs are still under development.

## MVP

Version 0.1 will prove this complete loop without leaving the terminal:

> browse → edit → save/format → build/test → inspect diagnostic → jump to source → fix → run

The MVP includes:

- Project and file tree
- Built-in non-modal UTF-8 editor with Go syntax highlighting
- File open/save, in-file search, and format-on-save with `gofmt`
- Asynchronous `go build ./...`, `go test ./...`, and `go run .`
- Integrated output and a shared diagnostic model
- Selection of `file:line:column` diagnostics and source navigation
- Discoverable keyboard shortcuts and function keys
- First-class SSH, tmux, terminal resize, and UTF-8 behavior

See the complete [MVP scope and acceptance test](docs/MVP.md) and [architecture](docs/ARCHITECTURE.md).

## Roadmap

- **v0.1:** editor, highlighting, project tree, formatting, build/test/run, and diagnostic navigation
- **v0.2:** gopls completion, diagnostics, definition, references, and hover
- **v0.3:** Delve breakpoints and interactive debugging

RapidGo will not manage Go versions. The user owns the Go toolchain; RapidGo owns only IDE-specific tooling it may add later.

## Run the terminal shell

Go 1.26 or newer is required:

```sh
make check
./build/rapidgo --version
./build/rapidgo .
```

The shell uses a VGA-inspired blue workspace with yellow text, white window titles, and cyan frames. Menus, the status bar, and help use light-gray surfaces with black labels and red shortcut hints. File and Help have framed dropdowns, and the current project path appears in the editor pane title. Press `F10` to open the menu, use Left/Right and Enter to choose an action, or use `Alt+F` and `Alt+H` to open a menu directly. Press `F1` to open or close shortcut help, `Esc` to close menus or help, and `Ctrl+Q` or `Ctrl+C` to quit. The project pane collapses on terminals narrower than 60 columns; resize redraws the layout. Terminal state is restored on exit.

## License

RapidGo is available under the [BSD 3-Clause License](LICENSE).
