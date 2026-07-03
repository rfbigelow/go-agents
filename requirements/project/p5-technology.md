# P5: Required Technology Elements

## Languages and Frameworks

### P5.1: Go 1.26+

**Purpose:** Implementation language for the library and all supporting code.
**Rationale:** Developer's language of choice. Strong concurrency primitives,
simple toolchain, and good fit for the library's goals.

### P5.2: Anthropic Go SDK

**Purpose:** Client library for communicating with the Anthropic Messages API.
**Rationale:** Official SDK from the LLM provider. Avoids reimplementing HTTP
client, authentication, and request/response serialization.

## Development Tools

### P5.3: Go Standard Toolchain

**Purpose:** Building, testing, linting, and formatting.
**Rationale:** Lean heavily on Go's built-in tools (`go test`, `go vet`,
`go fmt`, `go build`) rather than introducing external build systems or
task runners. Keeps the development workflow simple and dependency-light,
consistent with E3.2 (minimal external dependencies).

### P5.4: Go Module System

**Purpose:** Dependency management.
**Rationale:** Standard Go dependency management. No alternative needed.

### P5.5: GitHub Actions (CI and Release Automation)

**Purpose:** Continuous integration (gofmt, vet, build, test, version check)
and automated release cutting (tagging and publishing per P4.5).
**Rationale:** Native to the GitHub hosting already in use; no additional
infrastructure or credentials to manage. Workflows are plain YAML invoking
the standard toolchain (P5.3), keeping the automation dependency-light and
consistent with E3.2.
