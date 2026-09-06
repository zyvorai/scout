# Contributing

1. Fork the repository and create a focused branch.
2. Run `go test ./...` and `go vet ./...` before opening a PR.
3. Add tests for new rules, parsers, or API behavior.
4. Keep provider-specific code behind the discovery interface.
5. Never add credentials, exported user inventories, or proprietary compatibility data to tests.

By contributing, you agree that your contribution is licensed under Apache-2.0.
