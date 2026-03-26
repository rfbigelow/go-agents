# S2: Functionality

## Agent Lifecycle

### S2.1: Agent Creation and Configuration

**Description:** The consuming application creates an Agent and progressively
configures its capabilities. A minimally configured Agent can perform simple
LLM completions. Capabilities (tool use, human-in-the-loop, extended thinking,
deterministic logic) are added incrementally.
**Trigger:** Application initialization.
**Inputs:** Client instance, optional configuration (system prompt, model
parameters, capabilities).
**Outputs:** A configured Agent ready to run.
**Rules:** An Agent with no tools registered behaves as a simple chat
completion client. Adding tools enables the agentic conversation loop.
**Relates to:** G3.1 (reusability), E3.3 (platform agnosticism).

### S2.2: Conversation Loop Execution

**Description:** The Agent sends the current conversation state to the LLM
and processes the response. If the response contains tool-use requests, the
Agent dispatches them via the Tool Registry, appends the results to the
conversation, and repeats. The loop continues until the LLM produces a
final response with no tool-use requests.
**Trigger:** The consuming application initiates a run (e.g., by providing
a user message).
**Inputs:** User message, current conversation state.
**Outputs:** Final assistant response (streamed), updated conversation state.
**Rules:** All tool calls within a single LLM response are executed in parallel
before the next LLM turn. The turn completes when all tool calls (including
sub-agent invocations) finish. The loop must terminate (guard against infinite
tool-call cycles). The conversation loop is sequential at the turn level — a new
user message cannot be processed while a turn is in progress.
**Relates to:** S1.1 (Agent), S1.4 (Conversation State), S2.5 (Tool Dispatch),
S2.11 (Sub-Agent Composition).

### S2.3: Streaming Responses

**Description:** LLM responses are streamed to the consuming application as
they are generated, rather than waiting for the full response.
**Trigger:** Each LLM response during conversation loop execution.
**Inputs:** Streaming response from the Client.
**Outputs:** Incremental content delivered to the consuming application via
a callback or channel mechanism.
**Rules:** Streaming is the default mode. The consuming application must be
able to process partial responses.
**Relates to:** S1.2 (Client), G3.1 (reusability).

## Tool Use

### S2.4: Tool Registration

**Description:** The consuming application registers tool implementations
with the Agent. Each tool has a name, description, input schema, and an
execution function.
**Trigger:** Agent configuration, prior to running.
**Inputs:** Tool definition (name, description, input schema) and
implementation function.
**Outputs:** Tool is available for use by the LLM.
**Rules:** Tool names must be unique within an Agent. Tool definitions must
conform to the format expected by the Anthropic tool-use protocol.
**Relates to:** S1.3 (Tool Registry).

### S2.5: Tool Dispatch and Execution

**Description:** When the LLM requests a tool call, the Agent looks up the
tool by name in the registry and invokes it with the provided arguments.
The result is appended to the conversation as a tool result message.
**Trigger:** LLM response containing a tool-use block.
**Inputs:** Tool name and arguments from the LLM response.
**Outputs:** Tool result appended to conversation state.
**Rules:** All tool calls from a single LLM response execute in parallel.
Unknown tool names result in an error tool result sent back to the LLM (not a
crash). Tool execution errors are reported to the LLM as error results so it
can decide how to proceed.
**Relates to:** S1.3 (Tool Registry), S2.2 (Conversation Loop).

## Conversation Management

### S2.6: Conversation State Management

**Description:** The library maintains the message history for an agent session.
Messages are appended as the conversation progresses (user messages, assistant
responses, tool results). The library enforces correct message ordering and
tool-use protocol conventions.
**Trigger:** Each turn in the conversation loop.
**Inputs:** New messages generated during the conversation.
**Outputs:** Updated conversation state available for the next LLM call.
**Rules:** The library provides conversation state management with sensible
defaults. The consuming application can control resource-significant aspects
such as history bounds and compaction strategy (per E3.5) but does not need
to manage message ordering or protocol compliance.
**Relates to:** S1.4 (Conversation State), E3.5 (Consumer Resource Control).

## Resilience

### S2.7: Transient Error Handling

**Description:** The library handles transient API errors (rate limits,
network timeouts, server errors) with appropriate retry behavior.
**Trigger:** Transient error response from the Anthropic API.
**Inputs:** Error response.
**Outputs:** Retried request, or propagated error if retries are exhausted.
**Rules:** If the Anthropic Go SDK already provides retry behavior, the
library defers to it rather than layering additional retries. Non-transient
errors (authentication failures, invalid requests) are propagated
immediately.
**Relates to:** S1.2 (Client), E2.2 (Anthropic Go SDK).

## Progressive Capabilities

<!-- TODO: Detail these as they are further specified during elicitation. -->

### S2.8: Human-in-the-Loop

**Description:** The Agent can pause execution and request input or approval
from a human before continuing. This enables workflows where certain
decisions require human judgment.
**Trigger:** Agent-defined condition or tool that requires human input.
**Relates to:** S2.2 (Conversation Loop), E6.1 (Application Controls Execution
Flow).

<!-- TODO: Define the HITL execution model. Current thinking: the Agent's run
     method returns a response that indicates its type — either a final answer
     to the user's request or a HITL request for further user input. This keeps
     the execution model simple and leaves the application in control of the
     interaction flow (consistent with E6.1). Need to determine:
     - How the response type is represented (tagged union / enum + payload?)
     - How the application resumes the conversation after providing HITL input
     - Whether HITL responses carry structured metadata (e.g., what kind of
       input is needed) or are free-form text from the LLM -->

### S2.9: Extended Thinking

**Description:** The Agent supports Anthropic's extended thinking feature,
allowing the model to reason through complex problems before responding.
**Trigger:** Enabled via Agent configuration.
**Relates to:** S1.2 (Client), E2.1 (Anthropic Messages API).

### S2.10: Deterministic Logic

**Description:** The Agent can incorporate deterministic (non-LLM) logic
steps within a workflow — e.g., validation, transformation, or routing
that does not require LLM inference.
**Trigger:** Agent configuration includes deterministic steps.
**Relates to:** S2.2 (Conversation Loop).

### S2.11: Sub-Agent Composition

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
**Relates to:** S2.2 (Conversation Loop), S2.3 (Streaming), S2.5 (Tool
Dispatch), G5.4 (Composing Agents with Sub-Agents).
