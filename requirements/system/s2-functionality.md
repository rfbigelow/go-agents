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
completion client. A Tool Registry with registered tools enables the agent
loop.
**Relates to:** G3.1 (reusability), E3.3 (platform agnosticism), S2.15
(Conversation Resumption).

### S2.2: Agent Loop Execution

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
tool-call cycles). The agent loop is sequential at the turn level — a new
`run` cannot be initiated while a run is in progress.
**Relates to:** S1.1 (Agent), S1.4 (Conversation State), S2.5 (Tool Dispatch),
S2.11 (Sub-Agent Composition).

### S2.3: Streaming Responses

**Description:** LLM responses are streamed to the consuming application as
they are generated, rather than waiting for the full response.
**Trigger:** Each LLM response during agent loop execution.
**Inputs:** Streaming response from the Completer.
**Outputs:** Incremental content delivered to the consuming application via
a callback or channel mechanism.
**Rules:** Streaming is the default mode. The consuming application must be
able to process partial responses. Events may carry sub-agent attribution
(the sub-agent's name and nesting depth) when a sub-agent forwards its stream
to the parent (S2.11); top-level events carry empty attribution. When parallel
sub-agents forward to a shared stream, the library serializes delivery so the
callback is never invoked concurrently.
**Relates to:** S1.2 (Completer), S2.11 (Sub-Agent Composition), G3.1
(reusability).

### Agent ADT Stub

```
Types: AGENT

Creators:
  new_agent: COMPLETER × TOOL_REGISTRY × CONFIG → AGENT
  new_agent_with_history: COMPLETER × TOOL_REGISTRY × CONFIG × LIST[MESSAGE] → AGENT

Commands:
  run: AGENT × MESSAGE → AGENT
  with_hooks: AGENT × HOOK_BUNDLE → AGENT
  with_compaction: AGENT × COMPACTION_CONFIG → AGENT
  compact: AGENT → AGENT

Queries:
  messages: AGENT → LIST[MESSAGE]
  hooks: AGENT → HOOK_BUNDLE
  compaction: AGENT → COMPACTION_CONFIG
  usage: AGENT → TOKEN_USAGE

Preconditions:
  new_agent_with_history(c, r, cfg, msgs): msgs satisfies the five S2.15 invariants
```

The Agent's mutable state is its conversation history, its hook bundle, and
its compaction configuration. Each `run` appends the user message, drives the
agent loop (calling the Completer, dispatching tools, repeating as needed),
appends all resulting messages (assistant responses, tool results), and returns
the updated Agent. The response is delivered incrementally via the event stream
during the run. `run` leaves the hook bundle and compaction configuration
unchanged, but may compact the history if a configured trigger fires (S2.19).

`messages` returns the conversation history in the SDK-native message
representation (E1). It is the inverse of `new_agent_with_history`: the
list returned from `messages` may be persisted by the application and,
when later passed unchanged to `new_agent_with_history`, yields an Agent
whose conversation is equivalent to the original (S2.6, S2.15).

`new_agent_with_history` constructs an Agent whose initial conversation is the
supplied message list (S2.15). The precondition encodes the five resumption
invariants; S2.15 specifies how implementations report precondition violations
to the caller. `new_agent_with_history(c, r, cfg, [])` is equivalent to
`new_agent(c, r, cfg)`.

`with_hooks` replaces the Agent's hook bundle in full and returns the
updated Agent (S2.10). A HOOK_BUNDLE is a record carrying optional
handlers for the three hook points: `pre_llm_call`, `pre_tool_use`, and
`post_tool_use`. Any subset of handlers may be present; an empty bundle
(the default on newly-constructed Agents) means no hooks fire. There is
no partial-update command; replacing the bundle is the only way to change
hooks. Hooks may be set or replaced at any time, including between `run`
calls; the agent loop reads the current bundle at each hook point.

`with_compaction` replaces the Agent's compaction configuration in full and
returns the updated Agent (S2.18). A COMPACTION_CONFIG is a record carrying an
optional compaction strategy (S2.21, S3.6), optional trigger settings such as a
token threshold (S2.19), and an optional archival callback for the replaced
prefix (S2.18). The default on newly-constructed Agents is empty: no strategy,
so compaction never runs and the full history is retained (the library's default
behavior). `compact` applies the configured strategy to the committed history
immediately and returns the updated Agent; it is the explicit (manual) trigger
of S2.19 and is a no-op when no strategy is configured. `usage` returns the
conversation's token usage (S2.20).

**Configuration (CONFIG):**

| Parameter | Description |
|-----------|-------------|
| system | System prompt |
| model | Which model to use |
| max_tokens | Maximum tokens per LLM response |
| max_iterations | Maximum agent loop iterations before terminating (loop guard) |
| temperature | Sampling temperature (optional) |
| thinking | Extended thinking configuration (optional) — see S2.9 |
| effort | Output effort level (optional) — see S2.16 |

**Command-query table:**

```
                                        | messages                                            | hooks        | compaction    | usage
----------------------------------------+-----------------------------------------------------+--------------+---------------+-----------------------------------------------
new_agent                               | empty list                                          | empty bundle | empty         | zero
new_agent_with_history(c, r, cfg, msgs) | msgs (when validation passes)                       | empty bundle | empty         | zero
run(a, msg)                             | messages(a) + user message + agent loop messages,   | hooks(a)     | compaction(a) | usage(a) + this run's reported usage,
                                        |   compacted if a configured trigger fires (S2.19)   |              |               |   including any summarization call (S2.21)
with_hooks(a, hb)                       | messages(a)                                         | hb           | compaction(a) | usage(a)
with_compaction(a, cc)                  | messages(a)                                         | hooks(a)     | cc            | usage(a)
compact(a)                              | compacted history (S2.18), replaced prefix emitted  | hooks(a)     | compaction(a) | usage(a) + any summarization call (S2.21);
                                        |   to the archival callback; messages(a) if none set |              |               |   usage(a) for non-LLM strategies
```

`run` extends the conversation with the user message and all messages
produced during the agent loop — assistant responses, tool-use
requests, tool results — in protocol-correct order. If the loop involves
multiple turns (tool use), all intermediate messages are included. The `usage`
query reports cumulative token usage and is specified in S2.20.

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
- On denial, the agent loop continues — the LLM receives the denial as
  an error tool result and can adapt (try a different approach, ask for
  clarification, or produce a final response).
- When a turn contains one or more HITL-flagged tool calls, approval
  callbacks are invoked serially, in the order the tool calls appear in
  the LLM response. After all approval decisions are made, approved tools
  and non-HITL tools execute in parallel. Denied tools receive error
  results without executing.
- `run` does not return mid-loop for HITL decisions — the callback blocks
  the agent loop until it returns, consistent with how `run` blocks
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
- A sub-agent's HITL-flagged tools are gated by the parent's approval
  callback by default (S2.11), so a single human gate governs the whole
  agent tree. A sub-agent may instead be configured with its own approval
  callback. The callback can distinguish a sub-agent's tool calls from
  top-level calls.
**Relates to:** S2.4 (Tool Registration), S2.5 (Tool Dispatch), S1.3
(Tool Registry), S2.11 (Sub-Agent Composition), E6.1 (Application Controls
Execution Flow).

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

**Description:** Applications can register typed handlers that interpose on
specific points in the agent loop, allowing deterministic (non-LLM) logic
— validation, transformation, routing, caching, policy enforcement — to
influence loop behavior without going through the LLM. Three hook points
are defined: `PreLLMCall`, `PreToolUse`, and `PostToolUse`. Each hook point
has its own typed handler interface and its own typed decision return.

The Agent invokes the registered hook at each defined point and acts on the
returned decision. Hooks fire synchronously and block the loop until they
return.

**Trigger:** The application registers one or more hook handlers via
`with_hooks` (see Agent ADT); the corresponding event point is reached
during loop execution.
**Inputs:** Per-hook event payload — the message list and call parameters at
`PreLLMCall`, the tool name and arguments at `PreToolUse`, the tool result
at `PostToolUse`.
**Outputs:** A typed decision indicating how the loop should proceed:
`Continue` (proceed with the original payload), `Modify` (proceed with a
rewritten payload), `Substitute` (skip the underlying operation and use
the supplied synthetic result), or `Abort` (terminate the run, carrying
a reason value as the abort's payload). The handler signature is
`(Decision, error)`: a non-nil error return indicates the handler itself
malfunctioned and could not produce a decision; `Abort` is the only
decision that carries a reason — it expresses an *intentional* stop,
distinct from a malfunction.
**Rules:**
- Each hook point has its own typed handler interface and its own sealed
  decision type enumerating the moves legal at that point. `PostToolUse`
  does not offer `Substitute` — the tool has already executed.
- At most one handler may be registered per hook. Multi-handler composition
  is out of scope; observability composition is served by S2.3 (streaming),
  S2.12 (tracing), and S2.13 (logging).
- At `PreToolUse`, the deterministic hook fires before the HITL approval
  gate (S2.8). HITL is consulted only if the hook returned `Continue` or
  `Modify`. `Substitute` or `Abort` short-circuits before the human is
  bothered.
- When `PreToolUse` returns `Substitute`, the synthetic result still
  propagates through `PostToolUse` and through tracing/logging, carrying a
  `Synthesized: true` flag so observers can distinguish executed results
  from synthesized ones. The flag is sticky — a subsequent `Modify` at
  `PostToolUse` does not clear it.
- On `Abort`, the loop returns the reason carried by the `Abort` decision,
  wrapped to identify it as a hook-requested abort. Conversation state is
  preserved up to the last completed turn, matching the HITL panic
  invariant in S2.8. The partial turn that invoked the hook is not
  retained.
- A non-nil error returned alongside the decision indicates the handler
  malfunctioned (e.g., dependency unreachable, internal bug). The loop
  discards the decision and returns the error wrapped to identify it as a
  hook handler failure. Conversation state is preserved up to the last
  completed turn.
- If a hook handler panics, the Agent does not recover. The panic
  propagates as a fatal error from `run`, wrapped to identify it as a
  hook handler panic. State is preserved up to the last completed turn
  (same rule as S2.8).
- The library imposes no timeout on hook execution; cancellation is via
  the `context.Context` passed to `run`. Hooks are expected to be fast
  (machine-speed deterministic logic, not user prompts — see S2.8 for the
  human-latency contract).
- Per-turn-boundary observation is not within scope of S2.10. Applications
  needing turn-boundary signals use S2.12 (tracing) or S2.13 (logging);
  S2.3 emits chunk-granular events, not turn boundaries.
**Relates to:** S2.2 (Agent Loop), S2.5 (Tool Dispatch), S2.8 (HITL —
ordering at PreToolUse), S2.3 (Streaming), S2.12 (Tracing), S2.13
(Logging), S1.1 (Agent), S1.3 (Tool Registry).

#### S2.11: Sub-Agent Composition

**Description:** A tool can create and run a sub-agent — a separate agent
loop with its own conversation state, tools, and response stream. The parent
agent invokes a sub-agent as a tool call; the sub-agent runs to completion and
returns its result as the tool result. Multiple sub-agents can run in parallel
(as part of parallel tool execution within a turn).

A sub-agent is one-shot by default: each invocation runs a fresh sub-agent to
completion and discards its state. A sub-agent may opt in to multi-turn
operation, in which case the live sub-agent instance is retained across parent
tool calls and identified by a session handle. The first invocation mints the
handle and returns it alongside the result; a later invocation supplying that
handle resumes the same live instance, conversation history intact, so the
parent can steer the sub-agent over several turns. This retention is in-memory
and scoped to the parent run; it is distinct from S2.15 conversation resumption,
which reconstructs a top-level Agent from persisted history.
**Trigger:** The LLM requests a tool call whose implementation creates and runs
a sub-agent.
**Inputs:** Tool arguments passed to the sub-agent tool — at minimum the prompt
for the sub-agent, and, for multi-turn sub-agents, an optional session handle
identifying a prior invocation to resume.
**Outputs:** Sub-agent result returned as a tool result to the parent agent.
For multi-turn sub-agents, the result also carries the session handle so the
parent can resume the sub-agent on a subsequent turn. The sub-agent tool
documents this output convention in its description, so the parent model knows
how to parse the handle and resupply it.
**Rules:**
- Sub-agents cannot spawn further sub-agents (maximum nesting depth of one).
  This limit is enforced at runtime: a sub-agent tool invoked from within a
  sub-agent returns an error result rather than running.
- Each sub-agent has its own conversation state, independent of the parent's
  and of other sub-agents'.
- Each sub-agent produces its own response stream, separate from the parent's
  stream and from other sub-agents' streams, so that concurrent output can be
  rendered independently. By default a sub-agent's stream is isolated — its
  events are not delivered to the parent's event stream. A sub-agent may
  optionally forward its events to the parent's stream; forwarded events carry
  attribution (the sub-agent's name and nesting depth) so the application can
  distinguish them, and concurrent forwarding from parallel sub-agents is
  serialized (S2.3).
- Human-in-the-loop approval propagates to sub-agents: the parent's approval
  callback governs a sub-agent's HITL-flagged tools unless the sub-agent is
  configured with its own callback. The callback can distinguish a sub-agent's
  tool calls from the parent's (S2.8).
**Relates to:** S2.2 (Agent Loop), S2.3 (Streaming), S2.5 (Tool
Dispatch), S2.8 (Human-in-the-Loop), S2.15 (Conversation Resumption),
G5.4 (Composing Agents with Sub-Agents).

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
E6.1 (Application Controls Execution Flow), S2.2 (Agent Loop),
S2.5 (Tool Dispatch), S2.11 (Sub-Agent Composition), S2.17 (Prompt
Caching).

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
(Application Controls Execution Flow), S2.17 (Prompt Caching).

#### S2.17: Prompt Caching

**Description:** The Agent optimizes LLM requests by placing cache
control breakpoints on stable prefixes so the Anthropic API can cache
and reuse them. Three breakpoints are placed: on the system prompt, on
the tool definitions, and on the conversation history. The system and
tool breakpoints cache the static prefix that is identical across all
turns. The conversation breakpoint caches the growing message history,
benefiting both successive turns within a single `run` (agent loop)
and successive `run` calls across the conversation loop. Cache metrics
from the API response (cache_creation_input_tokens,
cache_read_input_tokens) are surfaced through the library's
observability channels: as span attributes on the `agent.llm_call` span
(S2.12) and as structured log attributes in the per-call log entry
(S2.13).
**Trigger:** Every LLM request built by the Agent during agent loop
execution.
**Inputs:** Agent configuration (prompt caching enabled by default,
opt-out via Config), system prompt, tool definitions, and conversation
messages from the current request.
**Outputs:** Cache control breakpoints on the last system block, last
tool definition, and second-to-last message in each request. Cache
token metrics on per-call spans and log entries.
**Rules:**
- **Static breakpoints.** The Agent places a cache control breakpoint
  on the last element of the system prompt array and on the last element
  of the tool definitions array. If only one is present, the breakpoint
  is placed on whichever is present.
- **Conversation breakpoint.** The Agent places a cache control
  breakpoint on the last content block of the second-to-last message in
  the messages array. This caches system + tools + all prior messages,
  excluding only the most recently added message. When there are fewer
  than two messages, no conversation breakpoint is placed.
- **Breakpoint budget.** The Anthropic API allows at most 4 cache
  control breakpoints per request. The library uses up to 3 (system,
  tools, conversation), leaving 1 slot of headroom.
- **Enabled by default.** Prompt caching is active unless the consuming
  application opts out via a configuration flag. When opted out, no cache
  control breakpoints are placed; cache metrics are still reported from
  the API response. The single flag controls all three breakpoints.
- **No conversation state mutation.** Breakpoints are applied to the
  request's copy of the messages, not to the stored conversation state.
  Reading the conversation after a `run` does not expose cache control
  markers.
- **Per-call metrics only.** Cache metrics are reported per LLM call, not
  accumulated across turns. Each `agent.llm_call` span carries
  `agent.cache_creation_input_tokens` and `agent.cache_read_input_tokens`
  as int64 attributes. Each per-call log entry includes
  `cache_creation_input_tokens` and `cache_read_input_tokens` as
  structured fields.
- **No event stream impact.** Cache metrics are not surfaced through the
  streaming event callback (S2.3). Applications needing programmatic
  access to cache metrics use tracing or log processing.
- **Pass-through semantics.** The library reports whatever cache metrics
  the API returns. It does not interpret, validate, or act on them.
**Relates to:** S1.1 (Agent), S1.2 (Completer), S2.2 (Agent Loop),
S1.4 (Conversation State), S2.6 (Conversation State Management),
S2.12 (Distributed Tracing), S2.13 (Structured Logging), S2.14
(Completer), E1 (Prompt Caching, Cache Control Breakpoint), E2.1
(Anthropic Messages API).

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
| stop_reason | Why the model stopped: end_turn, tool_use, max_tokens, stop_sequence, pause_turn, refusal |
| usage | Token counts: input, output, cache_creation_input, cache_read_input |

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

**Relates to:** S1.2 (Completer), S2.2 (Agent Loop), S2.3 (Streaming),
S2.17 (Prompt Caching), E2.1 (Anthropic Messages API), E2.2 (Anthropic
Go SDK).

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
**Relates to:** S1.3 (Tool Registry), S2.2 (Agent Loop), S2.8 (HITL).

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
**Trigger:** Each turn in the agent loop.
**Inputs:** New messages generated during the conversation.
**Outputs:** Updated conversation state available for the next LLM call.
**Rules:**
- The library provides conversation state management with sensible defaults.
  The consuming application can control resource-significant aspects such as
  history bounds and compaction strategy (per E3.5; the compaction mechanism is
  specified in S2.18) but does not need to manage message ordering or protocol
  compliance.
- The library exposes the conversation history through a read operation on
  the Agent (`messages`, see S2.1) that returns the messages in the
  SDK-native message representation (E1). This is the inverse of the
  resumption constructor: a list returned from `messages`, passed unchanged
  to `new_agent_with_history` (S2.15), yields an Agent whose conversation is
  equivalent to the original. Applications use this round-trip to persist
  and resume conversations using only previous types from the Anthropic Go
  SDK (E2.2), with no library-defined wrapper format.
- Committed conversation state never contains a turn with unresolved
  `tool_use` blocks. Tool implementation panics are recovered and
  converted to error `tool_result` blocks (S2.5), so their turns complete
  normally. Approval-callback panics (S2.8) are not recovered, but the
  partial turn is rolled back so committed state matches the last
  completed turn. Lists returned by `messages` therefore always satisfy
  the five S2.15 resumption invariants without read-side cleanup.
**Relates to:** S1.4 (Conversation State), S2.15 (Conversation Resumption),
S2.18 (Conversation Compaction), E3.5 (Consumer Resource Control).

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
the SDK-native message representation (E1) — the same form returned by the
Agent's `messages` query (S2.1, S2.6).
**Outputs:** A configured Agent whose conversation state is initialized to the
supplied history, ready to run; or an error identifying which validation rule
the history violates.
**Rules:**
- Construction validates the history against five invariants:
  1. The history is empty, or ends with an assistant message (so the next
     `run` can append a user message without breaking alternation).
  2. User and assistant messages alternate.
  3. Every assistant `tool_use` block has a matching `tool_result` block (by
     tool-use ID) in the immediately following user message.
  4. No `tool_result` block appears without a preceding `tool_use` for the
     same ID.
  5. If the history ends with an assistant message, that message contains
     no `tool_use` blocks. (A trailing assistant message with unresolved
     `tool_use` blocks is not resumable: the next operation must be a user
     message, but the protocol requires the next message to provide
     `tool_result` blocks for the unresolved IDs. Lists produced by
     `messages` (S2.6) satisfy this invariant by construction because the
     library does not commit partial turns; the invariant validates
     application-constructed histories.)
- Validation failure is a constructor error — no Agent is produced and no
  partial state is retained. The error identifies the failing rule.
- An empty history is valid; passing an empty history is equivalent to the
  basic constructor (S2.1).
- Resumption is for top-level Agents only. Sub-agents (S2.11) have
  independent state and are not constructed from prior history. A multi-turn
  sub-agent retains a live in-memory instance across parent tool calls (S2.11),
  but this is instance retention within a single parent run, not history-based
  reconstruction.
- The library checks structural and protocol-level invariants only. It does
  not verify that the history was produced by this library or by any
  particular model.
- The SDK-native message representation (E1) must remain the format
  returned by `messages` and accepted by `new_agent_with_history` as the
  library evolves. Future capabilities that change the in-memory
  representation (e.g., compaction per E1) must preserve round-tripping
  through that representation.
- Assistant messages may contain `thinking` blocks (S2.9). These are part
  of the SDK-native representation and must be persisted and restored
  verbatim, including their opaque `signature` fields. An assistant
  message that contains both `thinking` and `tool_use` blocks must be
  resumed with both intact; dropping its thinking blocks would produce a
  history that violates the Anthropic API's tool-use protocol on the next
  turn. The five invariants above are message-structure invariants and do
  not separately validate the content of thinking blocks.
**Relates to:** S1.4 (Conversation State), S2.1 (Agent Creation), S2.6
(Conversation State Management), S2.9 (Extended Thinking), S2.11
(Sub-Agent Composition), E3.5 (Consumer Resource Control), G4.4 (Manage
Conversation History).

### S2.18: Conversation Compaction

**Description:** When the consuming application configures a compaction strategy
(opt-in; off by default), the library reduces the size of the conversation
history so a long-running session can continue within the model's context
window. A compaction strategy is a transform over the committed message history
that produces a shorter replacement for a prefix of that history. The library
applies a strategy only at boundaries that preserve the protocol invariants
(S2.6, S2.15): it never splits a `tool_use`/`tool_result` pair and never drops
the `thinking` blocks of a retained tool-use turn (S2.9). With no strategy
configured, the library retains the full history unchanged (the library's
default behavior).
**Trigger:** A compaction trigger fires (S2.19) — proactive threshold, reactive
overflow, or the explicit `compact` command.
**Inputs:** The configured compaction strategy and archival callback (set via
`with_compaction`); the current committed history.
**Outputs:** A compacted conversation history; the replaced prefix delivered to
the archival callback; an updated Agent.
**Rules:**
- **Opt-in.** Compaction is disabled unless the consumer configures a strategy.
  Per E3.5 the library provides strategies (S2.21) but enables none by default
  and does not impose compaction.
- **Commitment is a property of the strategy.** A strategy whose output is
  non-deterministic or expensive — notably LLM summarization (S2.21) — must be
  *committed*: applied once, mutating the committed history, so the result is
  computed a single time, is stable across subsequent turns, and becomes a new
  cacheable prefix (S2.17). A strategy that is a pure, deterministic function of
  the history (e.g., sliding-window truncation) may instead be applied
  *transiently* to the outgoing request without mutating committed state,
  provided its output is stable across consecutive turns (its window advances in
  chunks, not every turn). A transient transform that would change the request
  prefix on every turn must be committed instead, to avoid invalidating the
  conversation cache (S2.17) on every call.
- **Invariant preservation.** The post-compaction committed history satisfies the
  S2.6 committed-state invariant and the five S2.15 resumption invariants with no
  read-side cleanup; `messages` (S2.6) on a compacted Agent still round-trips
  through `new_agent_with_history` (S2.15). Compaction cuts only on turn
  boundaries that keep every `tool_use` with its `tool_result` and retains the
  `thinking` blocks of any retained tool-use turn verbatim, including signatures
  (S2.9).
- **Archival of the replaced prefix.** Before a committed compaction discards the
  replaced prefix, the library delivers that prefix to a consumer-registered
  archival callback in the SDK-native representation (E1), so the application can
  persist it externally for lossless resume (S2.15). The library retains only the
  compacted working history (bounded memory, E3.5); external persistence of the
  archived prefix is the application's responsibility, as with conversation
  persistence under S2.6. If no archival callback is registered, the replaced
  prefix is discarded.
- **Caching.** A committed compaction invalidates the cached conversation prefix
  for one request (S2.17); the compacted prefix is cached on the following
  request. Strategies should therefore compact infrequently and substantially
  rather than trimming a little each turn.

**Relates to:** S1.4 (Conversation State), S2.6 (Conversation State Management),
S2.15 (Conversation Resumption), S2.17 (Prompt Caching), S2.9 (Extended
Thinking), S2.19 (Compaction Triggers), S2.21 (Library-Provided Compaction
Strategies), S3.6 (Compaction Strategy Interface), E3.5 (Consumer Resource
Control), G4.4 (Manage Conversation History).

### S2.19: Compaction Triggers

**Description:** Defines when the library applies a configured compaction
strategy (S2.18). Triggers are active only when a strategy is configured;
otherwise none fire and the full history is retained.
**Trigger:** Evaluated around each LLM call during the agent loop, and on the
explicit `compact` command.
**Inputs:** The configured strategy and trigger settings (e.g., a token
threshold); token usage (S2.20); the API's context-window overflow error.
**Outputs:** A compaction applied (or not) per the rules below.
**Rules:**
- **Manual.** The consumer can invoke compaction explicitly between runs via
  `compact` on the Agent ADT, independent of any automatic trigger.
- **Proactive (token threshold).** When the consumer configures a token
  threshold, the library applies the strategy before an LLM call once the
  conversation's token usage (S2.20) crosses that threshold, keeping the request
  within the context window proactively. The threshold may be expressed relative
  to the model's context window or as an absolute token count; with no threshold
  set, no proactive compaction occurs.
- **Reactive (on overflow).** When an LLM call fails because the request exceeds
  the model's context window (the overflow case of S4.1) and a strategy is
  configured, the library applies the strategy and retries the call once. If the
  request still overflows after one compaction, or if no strategy is configured,
  the library returns the error to the consumer without appending the offending
  message (S4.1). Reactive compaction is a fallback; the proactive trigger is the
  primary mechanism.

**Relates to:** S2.18 (Conversation Compaction), S2.20 (Token Usage Reporting),
S4.1 (Simple Conversation scenario), E3.5 (Consumer Resource Control).

### S2.20: Token Usage Reporting

**Description:** The library surfaces token usage to the consuming application so
it can drive compaction thresholds (S2.19) and reason about cost. Usage is
already captured from each API response and emitted to tracing and logging
(S2.12, S2.13); this requirement also exposes it programmatically through the
Agent.
**Trigger:** Each LLM call completes with usage data in the response.
**Inputs:** The `usage` the Anthropic API reports on each response — input
tokens, output tokens, cache-creation tokens, and cache-read tokens.
**Outputs:** Per-run and cumulative token usage readable by the consumer through
the `usage` query on the Agent.
**Rules:**
- After a `run`, the consumer can read the token usage attributable to that run
  and the cumulative usage for the conversation, broken down into input, output,
  and cache (creation/read) tokens.
- Reported usage reflects what the API returned, including the effect of prompt
  caching (S2.17) and of any compaction already applied (S2.18).
- Cumulative usage includes every Completer call the Agent makes on the
  conversation's behalf, including a summarizing strategy's own call (S2.21).
- Reporting is read-only and imposes no behavior by itself; the proactive
  trigger (S2.19) is what consumes a configured threshold.

**Relates to:** S2.19 (Compaction Triggers), S2.14 (Completer), S2.17 (Prompt
Caching), S2.12 (Distributed Tracing), S2.13 (Structured Logging).

### S2.21: Library-Provided Compaction Strategies

**Description:** The library ships ready-to-use compaction strategies (S2.18) so
consumers need not implement their own. None is enabled by default (S2.18 is
opt-in); the consumer selects a provided strategy or supplies a custom one
(S3.6).
**Trigger:** Consumer configures a provided strategy via `with_compaction`.
**Inputs:** The committed history; for summarizing strategies, the Agent's
Completer (S2.14) and model.
**Outputs:** A compacted history per S2.18.
**Rules:**
- **Hybrid summarization (recommended default).** Replaces an older prefix of the
  history with a single generated summary message and retains a recent verbatim
  window of turns. It invokes the Agent's Completer (S2.14) to produce the
  summary, so it is non-deterministic and is always committed (S2.18). The
  summary message and the retained window together satisfy the S2.15 invariants.
- **Sliding-window truncation.** Drops the oldest turns, cutting only on safe
  boundaries (S2.18), keeping a fixed recent window. It is deterministic, makes
  no LLM call, and may run transiently (S2.18) when its window advances in
  chunks.
- Selecting a provided strategy is opt-in and overridable; consumers can
  implement S3.6 directly for custom behavior, such as selective payload pruning
  that shrinks large `tool_result` bodies while keeping turn structure.

**Relates to:** S2.18 (Conversation Compaction), S2.14 (Completer), S2.17
(Prompt Caching), S3.6 (Compaction Strategy Interface), E3.5 (Consumer Resource
Control).
