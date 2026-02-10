# Contributing to Aetheris

Thank you for your interest in contributing to Aetheris!

## How to Contribute

1. Fork the repository
2. Create a feature branch (`git checkout -b my-feature`)
3. Commit your changes with clear messages
4. Push to your fork (`git push origin my-feature`)
5. Open a Pull Request

## Code Style

- Use `gofmt` for Go code formatting
- Run `golangci-lint` before submitting PRs
- Maintain existing module and package structure

### Git hooks

To run `gofmt` automatically on staged Go files before each commit, enable the project hooks (local to this repo only):

```bash
git config core.hooksPath .githooks
```

Or run the install script once: `./scripts/install-hooks.sh`

## Reporting Issues

- Check existing issues before creating a new one
- Provide steps to reproduce, expected behavior, and screenshots if applicable

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
