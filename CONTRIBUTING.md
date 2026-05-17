# Contributing to Vior

Thanks for your interest in contributing to Vior! This guide will help you get started.

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Bun](https://bun.sh/) (for desktop frontend)
- [Wails](https://wails.io/) (for desktop app)
- [gogio](https://gioui.org/doc/install) (for mobile app)

## Project Structure

```
cmd/vior/       CLI entry point (Cobra)
desktop/        Wails desktop app (Go + Svelte frontend)
mobile/         Gio mobile app
internal/       Shared packages (capture, stream, server, etc.)
docs/           Documentation and assets
```

## Setup

```bash
git clone https://github.com/subhashraveendran/Vior.git
cd Vior
go mod download
```

For the desktop app, install frontend dependencies:

```bash
cd desktop/frontend && bun install
```

## Building

```bash
# CLI only
make build

# Desktop app (requires Wails)
make desktop

# Mobile app (requires gogio)
cd mobile && go run .
```

## Running Tests

```bash
go test ./...
```

## Code Style

- Run `go fmt ./...` before committing.
- Run `go vet ./...` to catch common issues.
- Keep functions focused and packages small.

## Pull Request Process

1. Fork the repo and create a branch from `main`.
2. Make your changes with clear, descriptive commits.
3. Ensure `go vet` and `go test` pass.
4. Open a PR against `main` with a summary of what changed and why.
5. Address review feedback promptly.

## Reporting Issues

Use the [GitHub issue templates](https://github.com/subhashraveendran/Vior/issues/new/choose) for bug reports and feature requests.
