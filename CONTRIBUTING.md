# Contributing to Vior

Thanks for your interest in contributing to Vior! This guide will help you get started.

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Node.js 22+](https://nodejs.org/) (for desktop frontend and mobile)
- [Wails v2](https://wails.io/) (for desktop app)
- [Android SDK](https://developer.android.com/studio) (for mobile APK builds)

## Project Structure

```
cmd/vior/          CLI entry point (Cobra)
desktop/           Wails desktop app (Go + React/Vite frontend)
desktop/app.go     Wails bridge: all UI-callable backend methods
desktop/frontend/  React 19 + TypeScript frontend
internal/          Shared packages (capture, stream, protocol, etc.)
mobile-cap/        Capacitor 7 Android app (TypeScript in WebView)
docs/              Documentation and site assets
```

## Setup

```bash
git clone https://github.com/subhashraveendran/Vior.git
cd Vior
go mod download
```

For the desktop frontend:

```bash
cd desktop/frontend && npm install
```

For the mobile app:

```bash
cd mobile-cap && npm install
```

## Building

```bash
# CLI only
make build

# Desktop app (requires Wails)
make desktop

# Mobile APK (requires Android SDK)
cd mobile-cap && npm run build && npx cap sync && cd android && ./gradlew assembleDebug
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
