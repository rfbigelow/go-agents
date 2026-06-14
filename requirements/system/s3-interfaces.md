# S3: Interfaces

Since go-agents is a library (not a standalone application), its interfaces
are Go API surfaces consumed by application code.

## Library API

### S3.1: Agent API

**Type:** Go package API
**Consumers:** Agent application developers (G7.2).
**Key operations:** Create an Agent, configure capabilities, register tools,
run a conversation, receive streamed responses, read the conversation
history in the SDK-native message representation (for persistence and
resumption per S2.6, S2.15).
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

### S3.5: Sub-Agent API

**Type:** Go package API
**Consumers:** Agent application developers (G7.2) composing agents from
sub-agents (S2.11).
**Key operations:** Define a sub-agent declaratively (name, description, system
prompt, tool subset, model, max iterations, one-shot vs. multi-turn, optional
dedicated stream observer) and compile it into a Tool the parent registers like
any other (S3.2). For full control, hand-author a sub-agent Tool using the
exported building blocks instead.
**Key characteristics:** The declarative definition mirrors the Tool contract
(S3.2): the produced Tool's input schema accepts the sub-agent prompt (plus an
optional session handle for multi-turn sub-agents), and its result is the
sub-agent's final message. For multi-turn sub-agents the result's
session-handle format is documented in the generated tool description, so the
parent model can parse and resupply it (consistent with tool-use output-format
guidance). The library propagates the parent's event stream and
approval callback to the sub-agent through `context.Context` (consistent with
OTEL span propagation per S2.12), so the Tool execution signature is unchanged
and the parent's HITL gate and streaming reach the sub-agent without extra
wiring (S2.8, S2.3). The escape hatch exposes the propagated stream, approval
callback, and nesting depth, plus a stream-forwarding helper, so an application
can build a sub-agent Tool by hand while still inheriting these. Progressive
disclosure: the common case is one declarative call; full control is available
without it.

## External System Interface

### S3.4: Anthropic Messages API (via SDK)

**Type:** HTTP API (accessed through E2.2)
**Direction:** Bidirectional (request/response, streaming).
**Format:** JSON, as defined by the Anthropic API specification.
