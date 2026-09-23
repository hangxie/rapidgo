# Contributing to RapidGo

RapidGo is in its pre-alpha foundation phase. Before starting a change, check the [roadmap](docs/MVP.md) and open an issue for substantial design work.

## Development

Requirements:

- Go 1.26 or newer
- Git
- A UTF-8 terminal; tmux and an SSH-accessible Linux environment are useful for UI testing

Run the local quality gate:

```sh
make check
```

Build and inspect the current command:

```sh
make build
./build/rapidgo --version
./build/rapidgo .
```

Use Conventional Commit messages such as `feat: add project tree model` or `fix: preserve cursor column on resize`. Pull requests should explain the user-visible behavior and include tests for non-trivial state changes.

## Architecture

Read [Architecture](docs/ARCHITECTURE.md) before introducing packages or UI dependencies. In particular, editor state must remain independent from terminal rendering.

