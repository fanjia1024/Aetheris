# Contributing to Aetheris

Thank you for your interest in contributing to Aetheris.

## How to Contribute

1. **Fork** the repository on GitHub.
2. **Create a feature branch** from `main`: `git checkout -b feature/your-change`.
3. **Commit** your changes with clear, descriptive messages.
4. **Push** to your fork and **open a Pull Request** against the upstream `main` branch.

## Code Style and Quality

- Use **gofmt** for formatting: `gofmt -w .`
- Follow **Go Modules** for dependency management; run `go mod tidy` before committing.
- Run **go vet**: `go vet ./...`
- Prefer table-driven tests and clear test names.

For detailed build, lint, test commands, and project structure, see [AGENTS.md](AGENTS.md).

## Reporting Issues

- Please **search existing issues** before creating a new one.
- Use a clear title and describe the problem, steps to reproduce, and (if applicable) your environment (Go version, OS).
- For bug reports, include minimal code or config that reproduces the issue.

## Pull Requests

- Describe **what** you changed and **why** (e.g., “Fix scheduler lease timeout under load”).
- Reference any related **Issue** (e.g., “Fixes #123”).
- Ensure tests pass: `go test ./...`
- Keep changes focused; prefer several small PRs over one large one.

## CI and Automation

Automated checks (e.g., GitHub Actions) may be added in the future. Until then, please run `go build ./...`, `go vet ./...`, and `go test ./...` locally before submitting a PR.
