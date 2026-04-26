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

### S6.18: HITL Approval

**Verifies:** S2.8, S2.5
**Method:** Test with a HITL-flagged tool and an approval callback that
returns approve. Mock API returns a tool-use request for the HITL tool.
**Pass condition:** The approval callback is invoked with the correct tool
name and arguments. The tool executes normally after approval. The tool
result is sent back to the LLM.

### S6.19: HITL Denial

**Verifies:** S2.8, S2.5
**Method:** Test with a HITL-flagged tool and an approval callback that
returns deny. Mock API returns a tool-use request for the HITL tool.
**Pass condition:** The tool does not execute. An error tool result
indicating user denial is sent back to the LLM. The agentic loop
continues — the LLM receives the denial and can respond.

### S6.20: HITL Mixed Parallel Tool Calls

**Verifies:** S2.8, S2.5
**Method:** Test with a mix of HITL and non-HITL tools. Mock API returns
a response requesting both in a single turn. Approval callback approves
the HITL tool.
**Pass condition:** The HITL callback is invoked before any tool executes.
After approval, both tools execute in parallel. Results for both are sent
back to the LLM.

### S6.21: HITL Missing Callback Validation

**Verifies:** S2.4, S2.8
**Method:** Attempt to register a HITL-flagged tool with no approval
callback on the Tool Registry.
**Pass condition:** Registration fails with an error indicating that
an approval callback is required when HITL-flagged tools are present.

### S6.22: HITL Callback Panic Propagation

**Verifies:** S2.8, S4.5
**Method:** Test with a HITL-flagged tool and an approval callback that
panics. Mock API returns a tool-use request for the HITL tool.
**Pass condition:** The Agent does not recover the panic. `run` returns an
error representing the panic. Conversation state is preserved up to the turn
before the callback invocation — the partial turn (user message and LLM
tool-use response) is not retained. Log output includes the panic details
(per S2.13).

### S6.25: Extended Thinking

**Verifies:** S2.9
**Method:** Test with mocked API responses, exercising the multiple thinking
modes, the streaming surface, and multi-turn preservation.
**Pass condition:**
- An Agent configured with `thinking.type` set to `enabled`,
  `adaptive`, or `disabled` includes the configured value verbatim on each
  Completer request, with no library-side validation against the model.
  Configuring `thinking` with an unrecognized `type` is also passed through
  unchanged.
- When the application configures `thinking.type` as `enabled` or
  `adaptive` without specifying `display`, the request includes
  `display: "omitted"`. When the application sets `display: "summarized"`,
  that value is passed through. When the application sets
  `thinking.type: "disabled"`, the request omits `display` entirely.
- When the application does not configure `thinking`, the request omits the
  parameter.
- For a mocked streamed response containing a `thinking` content block, the
  event stream surfaces `thinking_delta` events (when the mock provides
  thinking text) and a `signature_delta` event before
  `content_block_stop`, in the order the mock emits them.
- After a turn whose mocked response contains both `thinking` and
  `tool_use` blocks, conversation state retains the thinking blocks
  (including their `signature` fields) within the assistant message, and
  the next Completer request — the one carrying the `tool_result` —
  includes those thinking blocks ahead of any later content, with
  signatures unchanged.
- For a turn whose response contains only `thinking` and `text` blocks, the
  thinking block is appended to conversation state with its signature
  unchanged.

### S6.26: Effort

**Verifies:** S2.16
**Method:** Test with mocked API responses.
**Pass condition:**
- An Agent configured with an `effort` value (e.g., `"low"`, `"medium"`,
  `"high"`, `"xhigh"`, `"max"`, or any other string) includes that value
  verbatim on each Completer request under `output_config.effort`, with no
  library-side validation against the model.
- An Agent with no `effort` configured omits the `output_config.effort`
  parameter from each Completer request.
- The presence or absence of a `thinking` configuration does not affect how
  `effort` is forwarded; effort and thinking are configured and propagated
  independently.

### S6.24: Conversation Resumption

**Verifies:** S2.15
**Method:** Test with mocked API responses, exercising both successful resume
and rejection of malformed histories.
**Pass condition:** An Agent constructed with a valid prior history (empty, or
ending with an assistant message and otherwise satisfying the four S2.15
invariants) initializes its conversation state to that history; a subsequent
`run` extends from the supplied history and the API request includes the prior
messages. An Agent constructed with each of the following malformed histories
yields a constructor error identifying the violated rule, and no Agent is
returned: (1) a history ending with a user message; (2) a history with two
consecutive same-role messages; (3) a history containing an assistant
`tool_use` block whose ID has no matching `tool_result` in the immediately
following user message; (4) a history containing a `tool_result` block whose
ID has no preceding `tool_use`. An empty history is accepted and yields an
Agent equivalent to one created via the basic constructor.

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

## Acceptance Test (Example Applications)

### S6.15: Tool-Use Example Application

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

### S6.23: HITL Example Application

**Verifies:** S2.8, and integration of S2.4 (registration with HITL flag) and
S2.5 (dispatch with approval gate).
**Method:** Demonstration — a separate example application exercising
tool-level human approval runs against the live Anthropic API.
**Pass condition:** Within a single session, the example exercises both an
approval path (the approval callback returns approve, the tool executes, and
its result is returned to the LLM) and a denial path (the callback returns
deny, a denial error is returned to the LLM, and the agentic loop continues to
a final response). The application surfaces each approval request to a human
with the tool name and arguments, and conveys the human's decision to the
callback. The application configures a slog handler so that approval-gate
decisions (including the "tool denied" record on the denial path) are
observable in structured logs. Trace observability is exercised by the
library's own instrumentation (S2.12) and verified by S6.16; the example does
not need to wire up an OTEL exporter, which would drown the interactive chat
loop in span output.

## Verification Coverage

| Requirement | Verified by |
|-------------|-------------|
| S2.1 | S6.1, S6.15 |
| S2.2 | S6.1, S6.3, S6.4, S6.8, S6.15 |
| S2.3 | S6.1, S6.15 |
| S2.4 | S6.2, S6.21, S6.15, S6.23 |
| S2.5 | S6.2, S6.5, S6.6, S6.7, S6.18, S6.19, S6.20, S6.15, S6.23 |
| S2.6 | S6.1, S6.15 |
| S2.7 | S6.8, S6.9 |
| S2.8 | S6.18, S6.19, S6.20, S6.21, S6.22, S6.23 |
| S2.9 | S6.25 |
| S2.10 | <!-- TODO: Add when deterministic logic is elaborated --> |
| S2.11 | S6.10, S6.11, S6.12 |
| S2.12 | S6.16, S6.14 |
| S2.13 | S6.17 |
| S2.15 | S6.24 |
| S2.16 | S6.26 |
