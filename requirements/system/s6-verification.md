# S6: Verification and Acceptance Criteria

## Verification Strategy

The library is verified at two levels:

- **Unit tests** with mocked API responses verify individual requirements.
  The Anthropic API (E2.1) is mocked to provide deterministic, repeatable
  test scenarios without network dependencies or API costs.
- **Example application** serves as acceptance test, verifying that the
  library's capabilities work together in a realistic agent application
  against the live Anthropic API.

## Unit Test Criteria

### S6.1: Simple Conversation

**Verifies:** S2.1, S2.2, S2.3, S2.6
**Method:** Test with mocked API responses.
**Pass condition:** An Agent created with a system prompt can send a message,
stream the mocked response, and maintain conversation history across multiple
turns. Conversation state contains the correct message sequence after each turn.

### S6.2: Tool Registration and Dispatch

**Verifies:** S2.4, S2.5
**Method:** Test with mocked API responses containing tool-use requests.
**Pass condition:** Registered tools are included in API requests. When the
mocked API returns tool-use blocks, the Agent dispatches to the correct tool
implementations and appends results to conversation state. Multiple tool calls
in a single response execute in parallel.

### S6.3: Agentic Loop Termination

**Verifies:** S2.2
**Method:** Test with mocked API responses that chain multiple tool-call turns
before a final response.
**Pass condition:** The Agent loops through tool-call/result turns and returns
when the mocked API produces a response with no tool-use requests. The final
response is returned to the caller.

### S6.4: Maximum Iteration Guard

**Verifies:** S2.2 (loop termination), S4.2 (max iterations reached)
**Method:** Test with mocked API responses that always request tool calls.
**Pass condition:** The Agent stops after the configured maximum iteration count
and returns an error to the caller. Conversation state is preserved up to the
last completed turn.

### S6.5: Unknown Tool Handling

**Verifies:** S2.5, S4.2 (unknown tool)
**Method:** Test with a mocked API response requesting a tool not in the registry.
**Pass condition:** The Agent sends an error tool result back to the LLM. The
agentic loop continues (no crash, no propagation to the consumer).

### S6.6: Tool Error Handling

**Verifies:** S2.5, S4.2 (tool returns error)
**Method:** Test with a tool implementation that returns an error.
**Pass condition:** The error is converted to an error tool result sent back to
the LLM. The agentic loop continues.

### S6.7: Tool Panic Recovery

**Verifies:** S2.5, S4.2 (tool panics)
**Method:** Test with a tool implementation that panics.
**Pass condition:** The Agent recovers the panic, converts it to an error tool
result, and continues the agentic loop. Other concurrent tool calls are
unaffected.

### S6.8: API Error Propagation

**Verifies:** S2.2, S2.7, S4.1 (API error)
**Method:** Test with mocked API returning non-transient errors.
**Pass condition:** The error is returned to the consumer. Conversation state
does not include the message that prompted the error. Prior conversation history
is preserved.

### S6.9: Transient Error Retry

**Verifies:** S2.7
**Method:** Test with mocked API returning transient errors followed by success
(or exhausting retries).
**Pass condition:** Transient errors are retried per the SDK's retry behavior.
If retries succeed, the conversation continues normally. If retries are exhausted,
the error propagates to the consumer per S6.8.

### S6.10: Sub-Agent Composition

**Verifies:** S2.11, S4.3
**Method:** Test with a tool that creates and runs a sub-agent using mocked API
responses.
**Pass condition:** The sub-agent runs its own agentic loop with its own
conversation state. Its result is returned as a tool result to the parent agent.
The parent's agentic loop continues.

### S6.11: Sub-Agent Failure Isolation

**Verifies:** S2.11, S4.3 (sub-agent error, sub-agent panic)
**Method:** Test with sub-agents that return errors and sub-agents that panic.
**Pass condition:** Failures are converted to error tool results for the parent.
The parent's agentic loop continues.

### S6.12: Sub-Agent Nesting Rejection

**Verifies:** S2.11, S4.3 (nesting attempt), G6.3
**Method:** Test with a sub-agent that attempts to spawn another sub-agent.
**Pass condition:** The nested spawn is rejected with an error. The sub-agent
receives an error tool result and can continue its own loop.

## Non-Functional Verification

### S6.13: Platform Agnosticism

**Verifies:** E3.3
**Method:** Inspection.
**Pass condition:** The library has no build tags, import paths, or runtime
checks specific to any OS, cloud platform, or deployment environment.

### S6.14: Minimal Dependencies

**Verifies:** E3.2
**Method:** Inspection of `go.mod`.
**Pass condition:** Direct dependencies are limited to the Go standard library,
the Anthropic Go SDK, and the OpenTelemetry Trace API. Any additional
dependency has explicit justification.

## Observability Verification

### S6.16: Trace Span Structure

**Verifies:** S2.12
**Method:** Test with an in-memory OTEL span exporter and mocked API
responses.
**Pass condition:** An agent run produces a span tree with the expected
structure: a root Agent.Run span with child spans for each LLM call and
tool dispatch. Tool dispatch spans have child spans for individual tool
executions. Sub-agent tool executions produce nested Agent.Run spans with
their own LLM call and tool spans as children. Spans carry expected
attributes (tool name, model, turn number). Failed operations record errors
on their spans.

### S6.17: Structured Log Output

**Verifies:** S2.13
**Method:** Test with a custom slog handler that captures log records, using
mocked API responses.
**Pass condition:** An agent run emits structured log entries at expected
lifecycle points (run started, LLM call, tool dispatched, run completed).
Log entries include expected contextual attributes (agent identifier, tool
name, turn number). Error events are logged at Error level. Debug-level
entries include operational detail. When tracing is active, log entries
include trace and span ID attributes.

## Acceptance Test (Example Application)

### S6.15: Example Application

**Verifies:** All Must-priority requirements (S5), E6.1 (application controls
execution flow).
**Method:** Demonstration — an example application runs against the live
Anthropic API.
**Pass condition:** The example application can hold a multi-turn conversation
with tool use, demonstrating that the library's core capabilities work together
in a realistic setting. The application controls the interaction loop — obtaining
user input, calling the Agent, and displaying responses. The application
configures an OTEL SDK exporter and slog handler, demonstrating that traces
and structured logs are emitted during agent execution.

## Verification Coverage

| Requirement | Verified by |
|-------------|-------------|
| S2.1 | S6.1, S6.15 |
| S2.2 | S6.1, S6.3, S6.4, S6.8, S6.15 |
| S2.3 | S6.1, S6.15 |
| S2.4 | S6.2, S6.15 |
| S2.5 | S6.2, S6.5, S6.6, S6.7, S6.15 |
| S2.6 | S6.1, S6.15 |
| S2.7 | S6.8, S6.9 |
| S2.8 | <!-- TODO: Add when HITL execution model is defined --> |
| S2.9 | <!-- TODO: Add when extended thinking is elaborated --> |
| S2.10 | <!-- TODO: Add when deterministic logic is elaborated --> |
| S2.11 | S6.10, S6.11, S6.12 |
| S2.12 | S6.16, S6.14 |
| S2.13 | S6.17 |
