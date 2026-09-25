# RapidGo

RapidGo is a lightweight, keyboard-first terminal IDE for Go. Linux, macOS, and Windows are the intended runtime platforms; BSD and other server Unix systems are best-effort targets. Android and iOS are not runtime targets, though they can be SSH clients to a machine running RapidGo. A typical remote workflow is:

```sh
ssh devbox
cd project
rapidgo .
```

The interface draws on the classic Borland DOS IDEs: Turbo Pascal's discoverable, keyboard-first visual style and Borland C++'s Project window. The project tree is not a Turbo Pascal 7 feature; TP7 managed projects through a primary file and project-specific configuration. RapidGo brings browsing, conventional non-modal editing, and build diagnostics together in one terminal application. It is intentionally not a Vim or Neovim configuration.

> [!IMPORTANT]
> RapidGo is pre-alpha. `rapidgo .` can browse, edit, search, and save UTF-8 files, with syntax highlighting for Go files. Go build/test/run jobs and diagnostic navigation are still under development.

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

The shell uses a VGA-inspired blue workspace with yellow text, white window titles, and cyan frames. Go keywords, strings, numbers, and comments use distinct colors; other files keep the normal yellow text. Menus, the status bar, and help use light-gray surfaces with black labels and red shortcut hints. File, Search, and Help have framed dropdowns.

The project tree starts at any directory; a `go.mod` marks a module when that directory is expanded, but a module is not required. In the tree, use Up/Down to select, Left/Right to collapse or expand, and Enter to expand a directory or open a file. Tree markers show loading (`[~]`), a retriable read error (`[!]`), and a symlink (`[@]`). Opening a file focuses the UTF-8 editor. Type to edit; use arrows, Home/End, PgUp/PgDn, or Ctrl+Home/End to move, Shift with movement to select, Backspace/Delete to remove text, Enter for a new line, Ctrl+A to select all, and Ctrl+Z/Y for undo/redo. Tab inserts a tab; Shift+Tab removes up to four leading spaces or one tab. Tabs display as four spaces without changing the buffer. `F6` or `Ctrl+F6` switches between the tree and editor, while `F3` returns to the tree. A `*` in the editor title marks unsaved edits. Switching files or quitting with edits asks for discard confirmation (`D`) or cancellation (`Esc`). The focused pane has a double-line border; inactive panes have single-line borders. On terminals narrower than 60 columns, only the focused pane is shown. Symlinks are shown but not followed, and files over 4 MiB are not opened. Filesystem reads run in the background; sorting and rendering very large directories can still pause the UI.

Press `F2` to save. Go files are formatted with the `gofmt` executable on your `PATH`; other UTF-8 files are saved without formatting. RapidGo writes a temporary sibling file before replacing the original, so a formatting or write error leaves the previous file intact and keeps the buffer dirty. It also refuses to overwrite a file whose contents changed on disk since it was opened. Editing can continue while saving, but switching files and quitting wait for the save to finish. If newer edits exist when it completes, they remain unsaved. `gofmt` output uses its own line-ending style; non-Go files retain the buffer's detected line-ending style.

Press `Ctrl+F` to enter a literal, case-sensitive search, then Enter to find or Esc to cancel. `Ctrl+G` finds the next occurrence and wraps to the start when needed. Matches are selected at whole-grapheme boundaries, including combining sequences and wide characters.

Press `F10` to open the menu, use Left/Right to choose a menu and Up/Down and Enter to choose an action, or use `Alt+F`, `Alt+S`, and `Alt+H` to open a menu directly. Press `F1` to open or close shortcut help, `Esc` to close menus or help, and `Ctrl+Q` or `Ctrl+C` to quit. Terminal resize redraws the layout, and terminal state is restored on exit.

## Keyboard reference

RapidGo borrows keys from the DOS Borland IDEs, but this is not yet a complete Borland keymap. The historical column follows the [Borland C++ 3.1 User's Guide](https://bitsavers.trailing-edge.com/pdf/borland/borland_C%2B%2B/Borland_C%2B%2B_Version_3.1_Users_Guide_1992.pdf) (Alternate command set unless marked CUA). The one TP7-specific entry comes from the [Turbo Pascal 7 User's Guide](https://turbopascal.nl/docs/Turbo_Pascal_Version_7.0_Users_Guide_1992.pdf). “Not assigned” means the shortcut does nothing in RapidGo today; it is not a promise about its eventual binding.

| Key | Historical action | RapidGo today |
| --- | --- | --- |
| `F1` | Help | Shortcut help |
| `F2` | Save | Save, formatting Go files with `gofmt` |
| `F3` | Open file | Focus project tree |
| `Alt+F3` | Close active window | Not assigned |
| `F4` | Run to cursor | Not assigned; debugger deferred |
| `F5` | Zoom/unzoom active window | Not assigned |
| `F6` | Next window | Switch tree/editor focus |
| `Ctrl+F6` | Next window (CUA) | Switch tree/editor focus |
| `Shift+F6` | Previous window in TP7; not listed in BC++ 3.1's window hot keys | Not assigned |
| `F7` | Trace into | Not assigned; debugger deferred |
| `F8` | Step over | Not assigned; debugger deferred |
| `F9` | Make | Not assigned; Go jobs not built yet |
| `Ctrl+F9` | Run | Not assigned; Go jobs not built yet |
| `F10` | Menu bar | Open/close menu bar |

Tab is deliberately not a global pane-switch key. The Borland editor used it for indentation, while dialogs used Tab and Shift+Tab to move among controls. RapidGo uses Tab to insert a tab in the editor and Shift+Tab to remove leading indentation.

## License

RapidGo is available under the [BSD 3-Clause License](LICENSE).
