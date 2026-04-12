# S2: Functionality

## Agent (S1.1)

### S2.1: Agent Creation and Configuration

**Description:** The consuming application creates an Agent by providing a
Completer, a Tool Registry, and configuration. A minimally configured Agent
(with an empty Tool Registry) performs simple LLM completions. Capabilities
(tool use, human-in-the-loop, extended thinking, deterministic logic) are
added incrementally.
**Trigger:** Application initialization.
**Inputs:** Completer instance, Tool Registry instance, configuration (system
prompt, model, max tokens, max iterations, optional parameters).
**Outputs:** A configured Agent ready to run.
**Rules:** An Agent whose Tool Registry is empty behaves as a simple chat
completion client. A Tool Registry with registered tools enables the agentic
loop.
**Relates to:** G3.1 (reusability), E3.3 (platform agnosticism).

### S2.2: Agentic Loop Execution

**Description:** The Agent sends the current conversation state to the LLM
and processes the response. If the response contains tool-use requests, the
Agent dispatches them via the Tool Registry, appends the results to the
conversation, and repeats. The loop continues until the LLM produces a
final response with no tool-use requests.
**Trigger:** The consuming application calls `run` with a user message.
**Inputs:** User message, current conversation state.
**Outputs:** Final assistant response (streamed), updated conversation state.
**Rules:** All tool calls within a single LLM response are executed in parallel
before the next LLM turn. The turn completes when all tool calls (including
sub-agent invocations) finish. The loop must terminate (guard against infinite
tool-call cycles). The agentic loop is sequential at the turn level — a new
`run` cannot be initiated while a run is in progress.
**Relates to:** S1.1 (Agent), S1.4 (Conversation State), S2.5 (Tool Dispatch),
S2.11 (Sub-Agent Composition).

### S2.3: Streaming Responses

**Description:** LLM responses are streamed to the consuming application as
they are generated, rather than waiting for the full response.
**Trigger:** Each LLM response during agentic loop execution.
**Inputs:** Streaming response from the Completer.
**Outputs:** Incremental content delivered to the consuming application via
a callback or channel mechanism.
**Rules:** Streaming is the default mode. The consuming application must be
able to process partial responses.
**Relates to:** S1.2 (Completer), G3.1 (reusability).

### Agent ADT Stub

```
Types: AGENT

Creators:
  new_agent: COMPLETER × TOOL_REGISTRY × CONFIG → AGENT

Commands:
  run: AGENT × MESSAGE → AGENT

Queries:
  conversation: AGENT → CONVERSATION_STATE
```

The Agent's mutable state is its conversation history. Each `run` appends
the user message, drives the agentic loop (calling the Completer,
dispatching tools, repeating as needed), appends all resulting messages
(assistant responses, tool results), and returns the updated Agent. The
response is delivered incrementally via the event stream during the run.

**Configuration (CONFIG):**

| Parameter | Description |
|-----------|-------------|
| system | System prompt |
| model | Which model to use |
| max_tokens | Maximum tokens per LLM response |
| max_iterations | Maximum agentic loop iterations before terminating (loop guard) |
| temperature | Sampling temperature (optional) |
| thinking | Extended thinking configuration (optional) |

**Command-query table:**

```
              | conversation
--------------+---------------------------------------------------------------
new_agent     | empty (no messages)
run(a, msg)   | conversation(a) + user message + agentic loop messages
```

`run` extends the conversation with the user message and all messages
produced during the agentic loop — assistant responses, tool-use
requests, tool results — in protocol-correct order. If the loop involves
multiple turns (tool use), all intermediate messages are included.

### Progressive Capabilities

<!-- TODO: Detail these as they are further specified during elicitation. -->

#### S2.8: Human-in-the-Loop

**Description:** Individual tools can be flagged as requiring human approval
during registration (S2.4). When the LLM requests a call to a HITL-flagged
tool, the Tool Registry invokes an approval callback before executing the
tool. The callback receives the tool name and arguments and returns a binary
decision: approve or deny.

The approval callback is registered with the Tool Registry. The Tool Registry
enforces that a callback is present if any HITL-flagged tools are registered
(fail-fast at registration time rather than at runtime).

The Agent does not know about HITL directly. It asks the Tool Registry to
dispatch tool calls; the registry handles the approval gate internally.

**Trigger:** The LLM requests a call to a tool flagged as requiring HITL
approval.
**Inputs:** Tool name and arguments, passed to the approval callback.
**Outputs:** If approved, the tool executes normally and its result is
returned. If denied, an error result indicating the user denied the action
is sent back to the LLM.
**Rules:**
- On denial, the agentic loop continues — the LLM receives the denial as
  an error tool result and can adapt (try a different approach, ask for
  clarification, or produce a final response).
- When a turn contains a mix of HITL and non-HITL tool calls, HITL callbacks
  are invoked first for all HITL-flagged tools. After all approval decisions
  are made, approved tools and non-HITL tools execute in parallel. Denied
  tools receive error results without executing.
- `run` always completes the agentic loop — HITL does not cause `run` to
  return mid-loop. The callback blocks the agentic loop until the human
  responds, consistent with how `run` blocks during LLM calls and tool
  execution.
- Plan-level HITL (approving a multi-step plan before execution) is an
  application concern, not a library concern. Applications can implement
  plan approval using a HITL-flagged tool (e.g., a "propose_plan" tool).
**Relates to:** S2.4 (Tool Registration), S2.5 (Tool Dispatch), S1.3
(Tool Registry), E6.1 (Application Controls Execution Flow).

#### S2.9: Extended Thinking

**Description:** The Agent supports Anthropic's extended thinking feature,
allowing the model to reason through complex problems before responding.
**Trigger:** Enabled via Agent configuration.
**Relates to:** S1.2 (Completer), E2.1 (Anthropic Messages API).

#### S2.10: Deterministic Logic

**Description:** The Agent can incorporate deterministic (non-LLM) logic
steps within a workflow — e.g., validation, transformation, or routing
that does not require LLM inference.
**Trigger:** Agent configuration includes deterministic steps.
**Relates to:** S2.2 (Agentic Loop).

#### S2.11: Sub-Agent Composition

**Description:** A tool can create and run a sub-agent — a separate agentic
loop with its own conversation state, tools, and response stream. The parent
agent invokes a sub-agent as a tool call; the sub-agent runs to completion and
returns its result. Multiple sub-agents can run in parallel (as part of parallel
tool execution within a turn).
**Trigger:** The LLM requests a tool call whose implementation creates and runs
a sub-agent.
**Inputs:** Tool arguments passed to the sub-agent tool, which uses them to
configure and run the sub-agent.
**Outputs:** Sub-agent result returned as a tool result to the parent agent.
**Rules:** Sub-agents cannot spawn further sub-agents (maximum nesting depth
of one). Each sub-agent has its own conversation state, independent of the
parent's. Each sub-agent produces its own response stream, separate from the
parent's stream and from other sub-agents' streams, so that concurrent output
can be rendered independently.
**Relates to:** S2.2 (Agentic Loop), S2.3 (Streaming), S2.5 (Tool
Dispatch), G5.4 (Composing Agents with Sub-Agents).

### Observability

#### S2.12: Distributed Tracing

**Description:** The library creates OpenTelemetry spans for significant
operations, forming a trace tree that represents the structure of an agent
run. Spans are created for: the overall Agent.Run invocation (root span for
top-level agents, child span for sub-agents),
each LLM API call, each tool dispatch batch, each individual tool execution,
and each sub-agent invocation. Spans carry attributes relevant to the
operation (e.g., tool name, model, turn number) and record errors when
operations fail. Span context is propagated via `context.Context`, so
sub-agent spans appear as children of the tool dispatch span that invoked
them.
**Trigger:** Every significant operation during agent execution.
**Inputs:** `context.Context` carrying the current span context.
**Outputs:** Spans exported via whatever OTEL SDK the consuming application
has configured. No-op if no SDK is configured.
**Rules:** The library depends only on the OTEL Trace API (E2.3), never on
the OTEL SDK. The consuming application is responsible for configuring the
OTEL SDK, choosing an exporter, and managing the tracer provider lifecycle.
Span names follow a consistent naming convention (e.g., `agent.run`,
`agent.llm_call`, `agent.tool.<name>`, `agent.sub_agent.<name>`).
**Relates to:** E2.3 (OTEL Trace API), E3.5 (Consumer Resource Control),
E6.1 (Application Controls Execution Flow), S2.2 (Agentic Loop),
S2.5 (Tool Dispatch), S2.11 (Sub-Agent Composition).

#### S2.13: Structured Logging

**Description:** The library emits structured log entries via slog at key
lifecycle points. Log entries include contextual attributes such as agent
identifier, tool name, turn number, and error details. Log levels follow
Go conventions: Info for lifecycle events (run started, run completed, tool
dispatched), Debug for operational detail (LLM request metadata, tool
arguments), Error for failures (API errors, tool errors, recovered panics).
The library obtains its logger from a `*slog.Logger` provided during Agent
configuration, defaulting to `slog.Default()` if none is provided.
**Trigger:** Key lifecycle events during agent execution.
**Inputs:** `*slog.Logger` provided at Agent configuration time.
**Outputs:** Structured log entries emitted via the configured slog handler.
**Rules:** The library never configures a slog handler — the consuming
application controls log output format, destination, and filtering. Log
messages are stable: message strings and attribute keys are not removed or
renamed without a major version bump, to support log-based alerting and
parsing. When OTEL tracing is active, log entries
include trace and span IDs as attributes to enable log-trace correlation.
**Relates to:** E2.4 (slog), E3.5 (Consumer Resource Control), E6.1
(Application Controls Execution Flow).

## Completer (S1.2)

### S2.14: Completer

**Description:** The Completer is the Agent's abstraction for LLM
communication. It is stateless and has a single operation: given a complete
request, return a streaming response. The Agent assembles the request
(messages, tool definitions, model configuration) and the Completer bridges
to the LLM API.

The Completer is created from an Anthropic client provided by the consuming
application. It acts as an Adapter: translating the request into the SDK's
`Messages.NewStreaming()` call and returning the resulting stream. It holds a
reference to the consumer-provided Anthropic client but maintains no state
of its own.

**ADT Stub:**

```
Types: COMPLETER

Creators:
  new_completer: ANTHROPIC_CLIENT → COMPLETER

Queries:
  complete: COMPLETER × COMPLETION_REQUEST → EVENT_STREAM
```

**Request parameters (COMPLETION_REQUEST):**

| Parameter | Description |
|-----------|-------------|
| messages | The conversation history (user, assistant, and tool-result messages) |
| model | Which model to use for this completion |
| max_tokens | Maximum tokens in the response |
| system | System prompt (optional) |
| tools | Tool definitions available for this completion (optional) |
| tool_choice | How the model should choose tools: auto, any, specific, or none (optional) |
| temperature | Sampling temperature (optional) |
| thinking | Extended thinking configuration (optional) |

**Response (EVENT_STREAM):**

The Completer returns an event stream that yields incremental events as the
LLM generates its response. The stream can be consumed event-by-event for
real-time streaming or accumulated into a complete response message. A
complete response message contains:

| Field | Description |
|-------|-------------|
| content | Ordered list of content blocks (text, tool use, thinking) |
| stop_reason | Why the model stopped: end_turn, tool_use, max_tokens |
| usage | Token counts (input, output) |

**Command-query table:**

Since the Completer is stateless with a single query and no commands or
creators, the standard command-query table is trivial:

```
              | complete
--------------+-------------------------------------------
(no commands) | —
```

The Completer has no commands because it has no mutable state. The
interesting structure is in the request and response types, documented
in the tables above.

**Relates to:** S1.2 (Completer), S2.2 (Agentic Loop), S2.3 (Streaming),
E2.1 (Anthropic Messages API), E2.2 (Anthropic Go SDK).

### S2.7: Transient Error Handling

**Description:** Transient API errors (rate limits, network timeouts, server
errors) are handled by the Anthropic Go SDK's built-in retry mechanism. The
library-provided Completer delegates to the SDK without adding its own retry
layer. The consuming application controls retry behavior by configuring the
Anthropic client it provides (e.g., `option.WithMaxRetries()`).
**Trigger:** Transient error response from the Anthropic API.
**Inputs:** Error response from the SDK.
**Outputs:** Either a successful response (if the SDK retried successfully)
or a propagated error (if retries were exhausted or the error is non-transient).
**Rules:** The library-provided Completer does not implement retry logic —
it relies on the SDK's retry behavior. Non-transient errors (authentication
failures, invalid requests) are propagated immediately. Custom Completer
implementations are responsible for their own error handling.
**Relates to:** S1.2 (Completer), E2.2 (Anthropic Go SDK).

## Tool Registry (S1.3)

### S2.4: Tool Registration

**Description:** The consuming application registers tool implementations
with the Tool Registry. Each tool has a name, description, input schema,
an execution function, and an optional HITL flag indicating the tool
requires human approval before execution (S2.8). The consuming application
can also register an approval callback with the Tool Registry to handle
HITL decisions.
**Trigger:** Application setup, prior to creating the Agent.
**Inputs:** Tool definition (name, description, input schema, HITL flag)
and implementation function. Optionally, an approval callback.
**Outputs:** Tool is available in the registry for use by the Agent.
**Rules:** Tool names must be unique within a Tool Registry. Tool definitions
must conform to the format expected by the Anthropic tool-use protocol. If
any tool is registered with the HITL flag, the registry must have an approval
callback registered — otherwise registration fails (fail-fast).
**Relates to:** S1.3 (Tool Registry), S2.8 (HITL).

### S2.5: Tool Dispatch and Execution

**Description:** When the LLM requests a tool call, the Agent looks up the
tool by name in the registry and invokes it with the provided arguments.
The result is appended to the conversation as a tool result message.
**Trigger:** LLM response containing a tool-use block.
**Inputs:** Tool name and arguments from the LLM response.
**Outputs:** Tool result appended to conversation state.
**Rules:** When a turn contains tool calls, the Tool Registry first invokes
the approval callback for any HITL-flagged tools (S2.8). After all approval
decisions are made, approved tools and non-HITL tools execute in parallel.
Denied tools receive error results without executing. Unknown tool names
result in an error tool result sent back to the LLM (not a crash). Tool
execution errors are reported to the LLM as error results so it can decide
how to proceed.
**Relates to:** S1.3 (Tool Registry), S2.2 (Agentic Loop), S2.8 (HITL).

## Conversation State (S1.4)

### S2.6: Conversation State Management

**Description:** The library maintains the message history for an agent session.
Messages are appended as the conversation progresses (user messages, assistant
responses, tool results). The library enforces correct message ordering and
tool-use protocol conventions.
**Trigger:** Each turn in the agentic loop.
**Inputs:** New messages generated during the conversation.
**Outputs:** Updated conversation state available for the next LLM call.
**Rules:** The library provides conversation state management with sensible
defaults. The consuming application can control resource-significant aspects
such as history bounds and compaction strategy (per E3.5) but does not need
to manage message ordering or protocol compliance.
**Relates to:** S1.4 (Conversation State), E3.5 (Consumer Resource Control).
