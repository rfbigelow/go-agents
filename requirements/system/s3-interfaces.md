# S3: Interfaces

Since go-agents is a library (not a standalone application), its interfaces
are Go API surfaces consumed by application code.

## Library API

### S3.1: Agent API

**Type:** Go package API
**Consumers:** Agent application developers (G7.2).
**Key operations:** Create an Agent, configure capabilities, register tools,
run a conversation, receive streamed responses.
**Key characteristics:** Idiomatic Go (context propagation, error returns,
interfaces). Progressive disclosure — simple use cases require minimal
configuration.

### S3.2: Tool Interface

**Type:** Go interface / struct
**Consumers:** Agent application developers (G7.2) implementing custom tools.
**Key operations:** Define a tool (name, description, input schema, optional
HITL flag), implement execution logic.
**Key characteristics:** The tool definition's input schema uses the Anthropic
Go SDK's native schema type (E2.2); the library does not interpose its own
schema representation. The execution function receives a `context.Context`
(for cancellation and OTEL span propagation per S2.12) and the raw tool-call
arguments as `json.RawMessage` — the tool author decodes into whatever Go
type and validator they prefer, keeping the library schema-agnostic at
dispatch time. The function returns a result string and an error; a non-nil
error surfaces to the LLM as an error tool result (S2.5), with sibling tool
calls in the same turn continuing unaffected. Simple contract — tool authors
should not need to understand library internals.

### S3.3: Completer API

**Type:** Go package API
**Consumers:** Agent component (S1.1).
**Key operations:** Create a Completer from an Anthropic client. Accept a
completion request and return a streaming response.
**Key characteristics:** Stateless Adapter — bridges between the Agent and
the Anthropic Go SDK (E2.2). The consuming application creates and owns the
Anthropic client; the Completer wraps it.

## External System Interface

### S3.4: Anthropic Messages API (via SDK)

**Type:** HTTP API (accessed through E2.2)
**Direction:** Bidirectional (request/response, streaming).
**Format:** JSON, as defined by the Anthropic API specification.
