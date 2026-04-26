# S2: Functionality

## Agent (S1.1)

### S2.1: Agent Creation and Configuration

**Description:** The consuming application creates an Agent by providing a
Completer, a Tool Registry, and configuration. A minimally configured Agent
(with an empty Tool Registry) performs simple LLM completions. Capabilities
(tool use, human-in-the-loop, extended thinking, deterministic logic) are
added incrementally. An Agent may alternatively be constructed with a prior
message history to resume a persisted session (S2.15).
**Trigger:** Application initialization.
**Inputs:** Completer instance, Tool Registry instance, configuration (system
prompt, model, max tokens, max iterations, optional parameters).
**Outputs:** A configured Agent ready to run.
**Rules:** An Agent whose Tool Registry is empty behaves as a simple chat
completion client. A Tool Registry with registered tools enables the agentic
loop.
**Relates to:** G3.1 (reusability), E3.3 (platform agnosticism), S2.15
(Conversation Resumption).

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
  new_agent_with_history: COMPLETER × TOOL_REGISTRY × CONFIG × LIST[MESSAGE] → AGENT ∪ ERROR

Commands:
  run: AGENT × MESSAGE → AGENT

Queries:
  conversation: AGENT → CONVERSATION_STATE

Preconditions:
  new_agent_with_history(c, r, cfg, msgs): msgs satisfies the four S2.15 invariants
```

The Agent's mutable state is its conversation history. Each `run` appends
the user message, drives the agentic loop (calling the Completer,
dispatching tools, repeating as needed), appends all resulting messages
(assistant responses, tool results), and returns the updated Agent. The
response is delivered incrementally via the event stream during the run.

`new_agent_with_history` constructs an Agent whose initial conversation is the
supplied message list (S2.15). When the precondition is violated, construction
yields an ERROR identifying the failing invariant rather than an Agent;
`new_agent_with_history(c, r, cfg, [])` is equivalent to `new_agent(c, r, cfg)`.

**Configuration (CONFIG):**

| Parameter | Description |
|-----------|-------------|
| system | System prompt |
| model | Which model to use |
| max_tokens | Maximum tokens per LLM response |
| max_iterations | Maximum agentic loop iterations before terminating (loop guard) |
| temperature | Sampling temperature (optional) |
| thinking | Extended thinking configuration (optional) — see S2.9 |
| effort | Output effort level (optional) — see S2.16 |

**Command-query table:**

```
                                        | conversation
----------------------------------------+---------------------------------------------------------------
new_agent                               | empty (no messages)
new_agent_with_history(c, r, cfg, msgs) | msgs (when validation passes)
run(a, msg)                             | conversation(a) + user message + agentic loop messages
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
- When a turn contains one or more HITL-flagged tool calls, approval
  callbacks are invoked serially, in the order the tool calls appear in
  the LLM response. After all approval decisions are made, approved tools
  and non-HITL tools execute in parallel. Denied tools receive error
  results without executing.
- `run` does not return mid-loop for HITL decisions — the callback blocks
  the agentic loop until it returns, consistent with how `run` blocks
  during LLM calls and tool execution. The library imposes no timeout on
  the callback; cancellation is via the `context.Context` passed to `run`.
- If the approval callback panics, the Agent does not recover. The panic
  propagates as a fatal error from `run`, and conversation state is
  preserved up to the last completed turn — the partial turn that invoked
  the callback is not retained. Rationale: a broken approval gate is a
  safety issue, not a recoverable tool-execution issue. Continuing the
  loop after a faulty gate would be worse than failing loudly.
- Plan-level HITL (approving a multi-step plan before execution) is an
  application concern, not a library concern. Applications can implement
  plan approval using a HITL-flagged tool (e.g., a "propose_plan" tool).
**Relates to:** S2.4 (Tool Registration), S2.5 (Tool Dispatch), S1.3
(Tool Registry), E6.1 (Application Controls Execution Flow).

#### S2.9: Extended Thinking

**Description:** The Agent exposes Anthropic's Extended Thinking feature
through Agent configuration. The application selects a thinking mode
(`enabled` with a `budget_tokens` cap, `adaptive` letting the model decide,
or `disabled`) and optionally a `display` setting (`summarized` or
`omitted`); the library forwards the configuration on each Completer
request, surfaces thinking events through the stream, and preserves
thinking blocks (including their opaque encrypted signatures) in
conversation state so multi-turn tool-use loops and resumed sessions remain
protocol-compliant.
**Trigger:** The application configures `thinking` on the Agent.
**Inputs:** `thinking` configuration — `type` (string, passed through),
optional `budget_tokens` (when `type` is `enabled`), and optional `display`
(when `type` is `enabled` or `adaptive`).
**Outputs:**
- The configured `thinking` value is included on each Completer request
  (S2.14).
- Thinking content arrives through the event stream (S2.3) as
  `thinking_delta` events (incremental thinking text) and a
  `signature_delta` event (the opaque signature, once per thinking block,
  immediately before `content_block_stop`).
- Thinking blocks are appended to the assistant message in conversation
  state (S2.6), preserving each block's `signature` verbatim.
**Rules:**
- **Pass-through validation.** The library does not validate
  `thinking.type`, `budget_tokens`, or `display` against the targeted
  model's supported set. Unsupported combinations surface as upstream API
  errors. This is consistent with how the library treats `model` (S2.1).
- **Library-side `display` default.** When the application sets
  `thinking.type` to `enabled` or `adaptive` without specifying `display`,
  the library sends `display: "omitted"` so the application is not exposed
  to per-model API defaults. Applications that surface thinking text in
  their UI must opt in by setting `display: "summarized"`. The library
  passes through whatever value the application sets. When `thinking.type`
  is `disabled`, the library does not send `display` (the API rejects
  `display` in combination with `disabled`).
- **No thinking by default.** When the application does not configure
  `thinking`, the library omits the parameter from the request and lets
  the model's API-side default apply.
- **Signature opacity and preservation.** The `signature` field is opaque
  to the library; it is never inspected, parsed, or mutated. Thinking
  blocks appended to conversation state retain their signatures and are
  resent verbatim on subsequent Completer requests.
- **Tool-use turn preservation.** When an assistant turn produces both
  thinking blocks and `tool_use` blocks, the thinking blocks remain part
  of that assistant message in conversation state. The next Completer
  request — the one carrying the corresponding `tool_result` — therefore
  includes the thinking blocks ahead of any later content, satisfying the
  Anthropic API's protocol requirement that thinking from a tool-use turn
  be preserved across the tool result.
- **Streaming surface.** The event stream (S2.3) yields `thinking_delta`
  and `signature_delta` events alongside `text_delta`, in the order the
  API produces them. Applications using `display: "summarized"` can render
  thinking text incrementally; applications using `display: "omitted"`
  observe only `signature_delta` (no thinking text) for each thinking
  block.
- **Tool choice constraint is the application's.** When thinking is
  enabled or adaptive, the Anthropic API restricts `tool_choice` to
  `auto` or `none`. The library does not pre-check this combination;
  invalid combinations surface as API errors.
- **Beta headers are the application's.** Anthropic beta headers related
  to thinking — most notably `interleaved-thinking-2025-05-14` for manual
  mode on models that require it — are configured on the Anthropic client
  (E2.2) by the application. The library does not manage thinking-related
  beta headers. (Adaptive thinking does not require a beta header on
  supported models.)
- **Persistence interaction.** Thinking blocks (including signatures) are
  part of the SDK-native message representation that S2.15 round-trips.
  A persisted assistant message that contains both thinking and `tool_use`
  blocks must be resumed verbatim; dropping its thinking blocks would
  produce a history that violates the Anthropic API's tool-use protocol on
  the next turn.
**Relates to:** S1.2 (Completer), S1.4 (Conversation State), S2.3
(Streaming), S2.6 (Conversation State Management), S2.14 (Completer),
S2.15 (Conversation Resumption), S2.16 (Effort), E1 (Extended Thinking,
Adaptive Thinking, Thinking Signature, Effort), E2.1 (Anthropic Messages
API), E2.2 (Anthropic Go SDK), G5.3 (Adding Extended Thinking).

#### S2.16: Effort

**Description:** The Agent exposes the Anthropic Messages API
`output_config.effort` parameter, which guides the model's overall token
allocation across an entire response — affecting text length, the number
and verbosity of tool calls, and (when Extended Thinking is configured)
the depth of reasoning. Effort is independent of Extended Thinking and may
be configured with `thinking.type` set to `enabled`, `adaptive`,
`disabled`, or unset.
**Trigger:** The application configures `effort` on the Agent.
**Inputs:** `effort` — a passed-through string value (e.g., `"low"`,
`"medium"`, `"high"`, `"xhigh"`, `"max"`).
**Outputs:** The configured `effort` is included on each Completer request
(S2.14).
**Rules:**
- **Pass-through validation.** The library does not validate effort values
  against the targeted model's supported set. Unsupported values surface
  as upstream API errors, consistent with how the library treats `model`
  (S2.1) and `thinking` (S2.9).
- **Independence from thinking.** Effort applies whether or not Extended
  Thinking is configured. With thinking `disabled` (or unset), effort
  still affects text and tool-call token allocation.
- **No library default.** When the application does not configure
  `effort`, the library omits the parameter from the request and lets the
  model's API-side default apply.
**Relates to:** S1.2 (Completer), S2.9 (Extended Thinking), S2.14
(Completer), E1 (Effort), E2.1 (Anthropic Messages API), G5.7 (Tuning
Output Effort).

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
| thinking | Extended thinking configuration (optional) — see S2.9 |
| effort | Output effort level (optional) — see S2.16 |

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
must conform to the format expected by the Anthropic tool-use protocol; the
library's tool definition wraps the Anthropic Go SDK's native input-schema
type (E2.2) rather than defining its own schema representation (see S3.2).
If any tool is registered with the HITL flag, the registry must have an
approval callback registered — otherwise registration fails (fail-fast).
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
how to proceed. Errors are isolated per tool call: a failure in one parallel
tool does not cancel its siblings — each call in the batch produces its own
result (success or error) independently. Panics in tool implementations are
recovered and reported as error tool results, with details logged (S2.13).
Tool executions inherit the `context.Context` of the enclosing `run`; the
library does not impose per-tool timeouts, leaving lifecycle control to the
consuming application.
**Relates to:** S1.3 (Tool Registry), S2.2 (Agentic Loop), S2.8 (HITL).

### Tool Registry ADT Stub

```
Types: TOOL_REGISTRY

Creators:
  new_registry → TOOL_REGISTRY

Commands:
  register: TOOL_REGISTRY × TOOL_DEFINITION → TOOL_REGISTRY
  set_approval_callback: TOOL_REGISTRY × APPROVAL_CALLBACK → TOOL_REGISTRY

Queries:
  definitions: TOOL_REGISTRY → LIST[TOOL_DEFINITION]
  has_approval_callback: TOOL_REGISTRY → BOOLEAN
  dispatch: TOOL_REGISTRY × LIST[TOOL_CALL] × CONTEXT → LIST[TOOL_RESULT]

Preconditions:
  register(r, t): ¬hitl(t) ∨ has_approval_callback(r)
```

The Tool Registry is effectively immutable after setup: the consuming
application registers tools and (optionally) an approval callback during
application initialization, then hands the registry to the Agent. `dispatch`
is modeled as a query because it does not mutate the registry itself — any
side effects it produces are attributable to the invoked tools'
implementations, not to the registry. The precondition on `register` encodes
the fail-fast rule from S2.4: a HITL-flagged tool may only be registered
once an approval callback is in place.

TOOL_DEFINITION and APPROVAL_CALLBACK are library-defined concepts with
glossary entries in E1. TOOL_CALL and TOOL_RESULT are Anthropic
tool-use-protocol types (E2.1) referenced as previous types; they have
glossary entries for readability but are not themselves specified by this
library. CONTEXT is the Go `context.Context` previous type carrying
cancellation and OTEL span propagation.

`hitl: TOOL_DEFINITION → BOOLEAN` is a projection on the previous type
TOOL_DEFINITION — it reads the HITL flag set at registration (S2.4). It
appears in the precondition on `register` and in the `dispatch` semantics
below; TOOL_DEFINITION itself is not specified as an ADT.

**Batch semantics.** `dispatch` takes the list of tool calls produced by a
single LLM turn, not one call at a time. Its behavior is two-phase, per S2.8:

1. **Approval phase.** For each call in `calls` in list order, if the call
   targets a HITL-flagged tool, the registry's approval callback is invoked
   synchronously. Callbacks are invoked serially — one at a time, in input
   order — and all approval decisions are collected before any tool
   executes.
2. **Execution phase.** Approved HITL tools and non-HITL tools execute in
   parallel. Denied HITL tools skip execution and produce an error tool
   result indicating denial. Unknown tool names produce an error tool result
   without executing anything.

**Result alignment.** The returned LIST[TOOL_RESULT] is 1:1 aligned with
the input LIST[TOOL_CALL] by index: `result[i]` is the outcome of
`calls[i]`. Every input call produces exactly one result — normal success,
tool error, denial error, or unknown-tool error. This alignment is what
lets callers reassemble tool-result messages in protocol-correct order
(S2.6).

`dispatch` is partial in one runtime case not visible in its signature:
if any approval callback panics during the approval phase, the panic
propagates out of `dispatch` as a fatal error and no TOOL_RESULTs are
returned for the batch (S2.8). This is distinct from tool-implementation
panics, which `dispatch` recovers per call and converts to error tool
results (S2.5).

**Command-query table:**

```
                             | definitions                    | has_approval_callback             | dispatch(r, calls, ctx)
-----------------------------+--------------------------------+-----------------------------------+------------------------------------------------------------
new_registry                 | empty                          | false                             | every call resolves to an unknown-tool error result; result list aligned by index with calls
register(r, t)               | definitions(r) appended with t | has_approval_callback(r) (unchanged) | calls matching t.name are handled by t (approval-gated when hitl(t), producing a denial error on deny); remaining calls behave as under dispatch(r, [call], ctx); two-phase batch ordering applies (see prose)
set_approval_callback(r, cb) | definitions(r) (unchanged)     | true                              | same tool matching and execution as dispatch(r, calls, ctx); HITL approvals use cb
```

**Relates to:** S1.3 (Tool Registry), S2.4 (Tool Registration), S2.5 (Tool
Dispatch), S2.8 (HITL).

## Conversation State (S1.4)

### S2.6: Conversation State Management

**Description:** The library maintains the message history for an agent session.
Messages are appended as the conversation progresses (user messages, assistant
responses, tool results). The library enforces correct message ordering and
tool-use protocol conventions; S2.15 formalizes those invariants at construction
time when a session is resumed from prior history.
**Trigger:** Each turn in the agentic loop.
**Inputs:** New messages generated during the conversation.
**Outputs:** Updated conversation state available for the next LLM call.
**Rules:** The library provides conversation state management with sensible
defaults. The consuming application can control resource-significant aspects
such as history bounds and compaction strategy (per E3.5) but does not need
to manage message ordering or protocol compliance.
**Relates to:** S1.4 (Conversation State), S2.15 (Conversation Resumption),
E3.5 (Consumer Resource Control).

### S2.15: Conversation Resumption

**Description:** The consuming application can create an Agent with a
pre-existing message history, so conversations persisted across process
boundaries can be resumed. The library validates the supplied history against
the same protocol invariants it enforces on its own appends (S2.6); on failure,
construction returns an error and no Agent is produced. Persistence itself —
serialization format on disk, storage backend, retention policy — is the
application's responsibility.
**Trigger:** Application initialization for a resumed session.
**Inputs:** Completer, Tool Registry, Config, and a list of prior messages in
the same SDK-native representation that the conversation read interface
returns (S2.6).
**Outputs:** A configured Agent whose conversation state is initialized to the
supplied history, ready to run; or an error identifying which validation rule
the history violates.
**Rules:**
- Construction validates the history against four invariants:
  1. The history is empty, or ends with an assistant message (so the next
     `run` can append a user message without breaking alternation).
  2. User and assistant messages alternate.
  3. Every assistant `tool_use` block has a matching `tool_result` block (by
     tool-use ID) in the immediately following user message.
  4. No `tool_result` block appears without a preceding `tool_use` for the
     same ID.
- Validation failure is a constructor error — no Agent is produced and no
  partial state is retained. The error identifies the failing rule.
- An empty history is valid; passing an empty history is equivalent to the
  basic constructor (S2.1).
- Resumption is for top-level Agents only. Sub-agents (S2.11) have ephemeral,
  independent state and are not constructed from prior history.
- The library checks structural and protocol-level invariants only. It does
  not verify that the history was produced by this library or by any
  particular model.
- The SDK-native message representation must remain the persistence wire
  format as the library evolves. Future capabilities that change the
  in-memory representation (e.g., compaction per E1) must preserve
  round-tripping through that representation.
- Assistant messages may contain `thinking` blocks (S2.9). These are part
  of the SDK-native representation and must be persisted and restored
  verbatim, including their opaque `signature` fields. An assistant
  message that contains both `thinking` and `tool_use` blocks must be
  resumed with both intact; dropping its thinking blocks would produce a
  history that violates the Anthropic API's tool-use protocol on the next
  turn. The four invariants above are message-structure invariants and do
  not separately validate the content of thinking blocks.
**Relates to:** S1.4 (Conversation State), S2.1 (Agent Creation), S2.6
(Conversation State Management), S2.9 (Extended Thinking), S2.11
(Sub-Agent Composition), E3.5 (Consumer Resource Control), G4.4 (Manage
Conversation History).
