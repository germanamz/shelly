# Shelly

A Go CLI application.

## Prerequisites

- [Go](https://go.dev/dl/) 1.25+
- [go-task](https://taskfile.dev/installation/) (task runner)

### Installing tools

Install all development tools at once:

```bash
go install mvdan.cc/gofumpt@latest
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install gotest.tools/gotestsum@latest
go install github.com/go-task/task/v3/cmd/task@latest
```

Make sure `$(go env GOPATH)/bin` is in your `PATH`:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Quick start

```bash
task build   # Build binary to ./bin/shelly
task run     # Run without building
./bin/shelly # Run the built binary
```

## Development

### Available tasks

Run `task --list` to see all tasks. Here's the full reference:

| Task | Description |
|------|-------------|
| `task build` | Build binary to `./bin/shelly` |
| `task run` | Run the application directly |
| `task fmt` | Format code with gofumpt |
| `task fmt:check` | Check formatting without writing (CI-friendly) |
| `task lint` | Run golangci-lint |
| `task lint:fix` | Run golangci-lint with auto-fix |
| `task test` | Run tests with gotestsum |
| `task test:coverage` | Run tests and print coverage by function |
| `task test:coverage:html` | Generate `coverage.html` report |
| `task check` | Run all checks: format, lint, test |

### Typical workflow

```bash
# Write code, then:
task fmt       # Format your changes
task lint:fix  # Fix lint issues automatically
task test      # Run tests

# Or run everything at once before pushing:
task check
```

## Tooling

| Tool | Purpose | Config |
|------|---------|--------|
| [gofumpt](https://github.com/mvdan/gofumpt) | Code formatting (strict superset of `gofmt`) | - |
| [golangci-lint v2](https://golangci-lint.run/) | Linting (50+ linters) | `.golangci.yml` |
| [testify](https://github.com/stretchr/testify) | Test assertions (`assert`, `require`) | - |
| [gotestsum](https://github.com/gotestyourself/gotestsum) | Formatted test output | - |
| [go-task](https://taskfile.dev/) | Task runner | `Taskfile.yml` |

### Linter configuration

The project uses golangci-lint v2 with the `standard` preset plus these additional linters:

- **gosec** - security-focused analysis
- **gocritic** - opinionated style/performance/bug checks
- **gocyclo** - cyclomatic complexity (threshold: 15)
- **unconvert** - unnecessary type conversions
- **misspell** - common spelling errors
- **modernize** - suggests modern Go idioms
- **testifylint** - catches testify misuse

See `.golangci.yml` for the full configuration.

## Project structure

```
.
├── main.go            # Application entry point
├── main_test.go       # Tests
├── go.mod             # Go module definition
├── go.sum             # Dependency checksums
├── Taskfile.yml       # Task runner config
├── .golangci.yml      # Linter config
└── CLAUDE.md          # Claude Code instructions
```

## Writing tests

Tests use the standard `testing` package with [testify](https://github.com/stretchr/testify) for assertions:

```go
func TestExample(t *testing.T) {
    got := doSomething()
    assert.Equal(t, "expected", got)
    assert.NoError(t, err)
}
```

Use `require` instead of `assert` when a failure should stop the test immediately:

```go
func TestExample(t *testing.T) {
    result, err := doSomething()
    require.NoError(t, err)  // stops here if err != nil
    assert.Equal(t, "expected", result)
}
```
