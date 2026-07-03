# Contributing to Breeze

First off, thank you for taking the time to contribute! ❤️

Breeze is built around one core philosophy:

> Maximum performance with a clean developer experience.

Every contribution is appreciated, whether it's a bug fix, documentation improvement, performance optimization, or a brand new feature.

---

## Getting Started

Fork the repository.

```bash
git clone https://github.com/nelthaarion/breeze.git

cd breeze

go mod download
go test ./...
```

---

## Development Principles

Please keep these principles in mind.

- Performance first.
- Avoid unnecessary allocations.
- Preserve zero-copy optimizations.
- Preserve lock-free fast paths where possible.
- Keep APIs simple and consistent.
- Write readable code.
- Add tests for new functionality.
- Keep public APIs backward compatible whenever possible.

---

## Pull Request Process

1. Fork the repository.
2. Create a feature branch.

```bash
git checkout -b feature/my-feature
```

3. Make your changes.
4. Run all tests.

```bash
go test ./...
```

5. Commit your changes.

```bash
git commit -m "feat: add websocket compression"
```

6. Push your branch.

```bash
git push origin feature/my-feature
```

7. Open a Pull Request.

---

## Commit Convention

Please use Conventional Commits.

Examples:

```
feat:
fix:
perf:
docs:
test:
refactor:
chore:
```

Example:

```
perf(router): reduce allocations during route lookup
```

---

## Code Style

- Follow standard Go formatting (`gofmt`)
- Keep functions focused
- Avoid unnecessary abstractions
- Prefer composition over inheritance
- Document exported APIs

---

## Reporting Bugs

Please include:

- Go version
- Operating System
- Breeze version
- Reproduction steps
- Expected behavior
- Actual behavior

---

## Feature Requests

Before opening a feature request:

- Search existing Issues.
- Explain the use case.
- Explain why it belongs inside the framework.

---

Thank you for helping make Breeze faster.