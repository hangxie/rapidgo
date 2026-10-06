# RapidGo

RapidGo is a lightweight, keyboard-first terminal IDE for Go. Linux, macOS, and Windows are the intended runtime platforms; BSD and other server Unix systems are best-effort targets. Android and iOS are not runtime targets, though they can be SSH clients to a machine running RapidGo. A typical remote workflow is:

```sh
ssh devbox
cd project
rapidgo .
```

The interface draws on the classic Borland DOS IDEs: Turbo Pascal's discoverable, keyboard-first visual style and Borland C++'s Project window. The project tree is not a Turbo Pascal 7 feature; TP7 managed projects through a primary file and project-specific configuration. RapidGo brings browsing, conventional non-modal editing, and build diagnostics together in one terminal application. It is intentionally not a Vim or Neovim configuration.

![RapidGo's VGA color theme with the project tree, a Go file, and program output](docs/images/editor.png)

## Quick start

You need Go 1.26 or newer on your `PATH`. `gopls` is optional and enables completion, live problems, navigation, and hover.

```sh
go install github.com/hangxie/rapidgo/cmd/rapidgo@latest
go install golang.org/x/tools/gopls@latest  # optional
rapidgo path/to/project                     # or run rapidgo inside the project
```

Then, inside RapidGo:

| Key | Action |
| --- | --- |
| `F1` | List every shortcut |
| Up/Down, Enter | Choose a file in the project tree and open it |
| `F2` | Save, formatting Go files with `gofmt` |
| `F9` / `Ctrl+T` / `Ctrl+F9` | Build, test, or run |
| `Ctrl+F6`, then Enter | Move to the bottom pane and jump to a reported error |
| `F10` | Open the menus, which list every action |
| `Ctrl+Q` | Quit |

See the [user guide](docs/GUIDE.md) for every feature and shortcut, and [Install](#install) for prebuilt binaries and building from a checkout.

## MVP

Version 0.1 covers this complete loop without leaving the terminal:

> browse → edit → save/format → build/test → inspect diagnostic → jump to source → fix → run

The MVP includes:

- Project and file tree
- Built-in non-modal UTF-8 editor with Go syntax highlighting
- File open/save, in-file search, and format-on-save with `gofmt`
- Asynchronous `go build ./...`, `go test -json ./...`, and `go run` on a resolved main package
- Integrated output and a shared diagnostic model
- Selection of `file:line:column` diagnostics and source navigation
- Discoverable keyboard shortcuts and function keys
- First-class SSH, tmux, terminal resize, and UTF-8 behavior

See the complete [MVP scope and acceptance test](docs/MVP.md) and [architecture](docs/ARCHITECTURE.md).

## Screenshots

gopls problems appear in the gutter and in the Errors view; Enter jumps to the source.

```text
  File  Search   Build   Window   Help
┌─ PROJECT [module] ─┐ ┌─ main.go [gopls 1 error, 0 warning] ────────────────────────────┐
│[-] demo [mod]      │ │   5     "os"                                                    │
│  [-] cmd           │ │   6                                                             │
│    [-] greeter     │ │   7     "example.com/greeter/greet"                             │
│          main.go   │ │   8 )                                                           │
│  [+] greet         │ │   9                                                             │
│      go.mod        │ │  10 func main() {                                               │
│                    │ │  11     g := greet.Greeter{Mark: "!"}                           │
│                    │ │  12     names := os.Args[1:]                                    │
│                    │ │  13     if len(names) == 0 {                                    │
│                    │ │  14         names = []string{"Gopher", "World"}                 │
│                    │ │  15     }                                                       │
│                    │ │  16!    fmt.Println(g.Helo(names...))                           │
└────────────────────┘ └─────────────────────────────────────────────────────────────────┘
╔═ ERRORS  1 diagnostic(s) ══════════════════════════════════════════════════════════════╗
║cmd/greeter/main.go:16:16 [error] g.Helo undefined (type greet.Greeter has no field or m║
║                                                                                        ║
║                                                                                        ║
║                                                                                        ║
║                                                                                        ║
║                                                                                        ║
║                                                                                        ║
║                                                                                        ║
║                                                                                        ║
║                                                                                        ║
║                                                                                        ║
╚════════════════════════════════════════════════════════════════════════════════════════╝
 gopls 16:16: g.Helo undefined (type greet.Greeter has no field or method Helo)
 F2 Save  Ctrl+F Find  F3 Tree  F6 Window  F10 Menu  F1 Help  Ctrl+Q Quit
```

`Alt+I` shows gopls hover information for the symbol under the caret.

```text
  File  Search   Build   Window   Help
┌─ PROJECT [module] ─┐ ╔═ main.go [gopls ready] ═════════════════════════════════════════╗
│[-] demo [mod]      │ ║   5     "os"                                                    ║
│  [-] cmd           │ ║   6                                                             ║
│    [-] greeter     │ ║   7     "example.com/greeter/greet"                             ║
│          main.go   │ ║   8 )                                                           ║
│  [+] greet         │ ║   9                                                             ║
│      go.mod        │ ║  10 func main() {                                               ║
│                    │ ║  11     g := greet.Greeter{Mark: "!"}                           ║
│                    │ ║  12     names := os.Args[1:]                                    ║
│                    │ ║  13     if len(names) == 0 {                                    ║
│                    │ ║  14         names = []string{"Gopher", "World"}                 ║
│        ┌──────────────────────────────────────────────────────────────────────┐        ║
│        │ gopls hover                                                          │        ║
│        │ func (g greet.Greeter) Hello(names ...string) string                 │        ║
│        │ Hello greets each name in turn.                                      │        ║
│        │ Esc close  Up/Down scroll                                            │        ║
│        └──────────────────────────────────────────────────────────────────────┘        ║
│                    │ ║                                                                 ║
│                    │ ║                                                                 ║
│                    │ ║                                                                 ║
└────────────────────┘ ╚═════════════════════════════════════════════════════════════════╝
┌─ OUTPUT  go build ./... (succeeded) ───────────────────────────────────────────────────┐
│                                                                                        │
│                                                                                        │
│                                                                                        │
│                                                                                        │
└────────────────────────────────────────────────────────────────────────────────────────┘
 Hover: Esc closes; arrows and PgUp/PgDn scroll
 F2 Save  Ctrl+F Find  F3 Tree  F6 Window  F10 Menu  F1 Help  Ctrl+Q Quit
```

Additional files open as cascaded windows; Window → Tile arranges them side by side.

```text
  File  Search   Build   Window   Help
┌─ PROJECT [module] ─┐ ┌─ main.go ──────────┐┌─ greet.go ─────────┐╔═ go.mod ════════════╗
│[-] demo [mod]      │ │   5     "os"       ││   1 // Package gree│║   1 module example.c║
│  [-] cmd           │ │   6                ││   2 package greet  │║   2                 ║
│    [-] greeter     │ │   7     "example.co││   3                │║   3 go 1.26         ║
│          main.go   │ │   8 )              ││   4 import (       │║   4                 ║
│  [-] greet         │ │   9                ││   5     "fmt"      │║                     ║
│        greet.go    │ │  10 func main() {  ││   6     "strings"  │║                     ║
│      go.mod        │ │  11     g := greet.││   7 )              │║                     ║
│                    │ │  12     names := os││   8                │║                     ║
│                    │ │  13     if len(name││   9 // Greeter form│║                     ║
│                    │ │  14         names =││  10 type Greeter st│║                     ║
│                    │ │  15     }          ││  11     Mark string│║                     ║
│                    │ │  16     fmt.Println││  12 }              │║                     ║
│                    │ │  17 }              ││  13                │║                     ║
│                    │ │  18                ││  14 // Hello greets│║                     ║
│                    │ │                    ││  15 func (g Greeter│║                     ║
│                    │ │                    ││  16     parts := ma│║                     ║
│                    │ │                    ││  17     for _, name│║                     ║
│                    │ │                    ││  18         parts =│║                     ║
│                    │ │                    ││  19     }          │║                     ║
└────────────────────┘ └────────────────────┘└────────────────────┘╚═════════════════════╝
┌─ OUTPUT  go build ./... (succeeded) ───────────────────────────────────────────────────┐
│                                                                                        │
│                                                                                        │
│                                                                                        │
│                                                                                        │
└────────────────────────────────────────────────────────────────────────────────────────┘
 go build ./... succeeded
 F2 Save  Ctrl+F Find  F3 Tree  F6 Window  F10 Menu  F1 Help  Ctrl+Q Quit
```

## Language

RapidGo selects its language from `LC_ALL`, then `LC_MESSAGES`, then `LANG`, with `en_US` as the fallback. Common UI text is also available in Simplified Chinese (`zh_CN`). See [locale selection and adding language packs](docs/I18N.md).

## Roadmap

- **v0.1 (released):** editor, highlighting, project tree, formatting, build/test/run, and diagnostic navigation
- **v0.2 (released):** gopls completion, diagnostics, definition, references, and hover
- **v0.3 (planned):** Delve breakpoints and interactive debugging

RapidGo will not manage Go versions. The user owns the Go toolchain; RapidGo owns only IDE-specific tooling it may add later.

## Install

Install the latest release with Go 1.26 or newer:

```sh
go install github.com/hangxie/rapidgo/cmd/rapidgo@latest
rapidgo --version
rapidgo .
```

Go installs the binary into `GOBIN`, or `GOPATH/bin` if `GOBIN` is unset; add that directory to your `PATH` if `rapidgo` is not found. The installed Go toolchain remains necessary for RapidGo's build, test, run, and format actions, and `gopls` must be on your `PATH` for completion, problem reports, navigation, and hover. A version installed with `go install` reports its module version; builds made with `make build` also report the commit and build time.

Prebuilt binaries are available on the [releases page](https://github.com/hangxie/rapidgo/releases). Choose the archive for your OS and architecture, extract it, rename the extracted binary to `rapidgo` (or `rapidgo.exe` on Windows), and place it on your `PATH`. Release assets include SHA-512 checksums in `checksum-sha512.txt`. You still need a local Go installation for RapidGo's build, test, run, and format actions.

To build from a checkout:

```sh
make check
./build/rapidgo --version
./build/rapidgo .
```

To prepare release assets locally from a version tag, run `make release-build`. This writes platform archives, a license copy, and checksums to `build/release/`, plus release notes to `build/CHANGELOG`. Pushing a `vMAJOR.MINOR.PATCH` tag runs the quality gate, builds these assets, and publishes a GitHub release.

## Documentation

- [User guide](docs/GUIDE.md): every feature, menu, and shortcut
- [MVP scope and acceptance test](docs/MVP.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Locales and language packs](docs/I18N.md)

## License

RapidGo is available under the [BSD 3-Clause License](LICENSE).
