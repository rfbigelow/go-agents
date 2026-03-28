# E2: Environment Components

## E2.1: Anthropic Messages API

**Type:** API
**Description:** The LLM completion API provided by Anthropic. Supports
multi-turn conversation, tool use, and streaming. Accessed via the Anthropic
Go SDK.
**Interaction:** The library sends conversation messages (including tool
results) and receives model completions (including tool-use requests).
**Owner:** Anthropic. API changes are versioned but could affect the library
through SDK updates.

## E2.2: Anthropic Go SDK

**Type:** Library dependency
**Description:** Anthropic's official Go client library for the Messages API.
Handles authentication, request construction, and response parsing.
**Interaction:** The library depends on the SDK as its primary external
dependency. Agent applications interact with Anthropic through the SDK via
the library's abstractions.
**Owner:** Anthropic.

## E2.3: OpenTelemetry Trace API

**Type:** Library dependency
**Description:** The Go implementation of the OpenTelemetry Trace API
(`go.opentelemetry.io/otel/trace`). Provides the interfaces for creating
and managing spans. The API package is lightweight and acts as a no-op
when no OTEL SDK is configured by the consuming application.
**Interaction:** The library creates spans via the Trace API to represent
agent runs, LLM calls, tool executions, and sub-agent invocations. The
consuming application optionally configures an OTEL SDK and exporter to
collect and ship these traces.
**Owner:** OpenTelemetry project (CNCF).

## E2.4: Go Standard Library slog Package

**Type:** Standard library
**Description:** The structured logging package in the Go standard library
(`log/slog`). Provides leveled, structured logging with pluggable handlers.
**Interaction:** The library emits structured log entries at key lifecycle
points. The consuming application controls the slog handler (and therefore
log output format, destination, and filtering).
**Owner:** Go project. Part of the standard library since Go 1.21.
