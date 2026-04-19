# E1: Glossary

## Terms

### Agent

A software system in which an LLM autonomously drives a multi-step workflow,
making decisions about which actions to take based on context and intermediate
results. Distinguished from a simple LLM chat interaction by its ability to
use tools and pursue goals across multiple turns.

### Harness

The reusable runtime infrastructure that manages an agent's execution: the
agentic loop, tool dispatch, error handling, and interaction with the
LLM API. The harness is what this library provides; agent-specific behavior
is layered on top of it.

### Tool

A function or capability made available to the agent (via the LLM's tool-use
protocol) that allows it to take actions beyond generating text — e.g.,
reading files, making API calls, or executing commands.

### Tool Definition

The library's description of a tool as registered in the Tool Registry:
the name used by the LLM to invoke it, a description, an input schema
conforming to the Anthropic tool-use protocol (using the SDK's native
schema type per E2.2), an optional HITL flag, and the execution function.
Distinguished from a Tool Call (the LLM's invocation) and a Tool Result
(the dispatched response).

### Tool Call

A request from the LLM to invoke a tool, as delivered by the Anthropic
tool-use protocol (E2.1). Carries the tool name and the arguments as
produced by the model. The Tool Registry dispatches tool calls to the
registered implementation.

### Tool Result

The response to a Tool Call, fed back to the LLM in the subsequent
conversation turn. A successful result carries the tool's output; an
error result carries an error indication (from an execution failure, a
recovered panic, an unknown tool name, or a HITL denial) so the LLM can
adapt.

### Agentic Spectrum

The range of agent sophistication, from simple single-turn LLM completions
(no tools, no loops) through tool-using agents to fully autonomous multi-step
workflows with human-in-the-loop control. The library is designed to support
applications at any point on this spectrum.

### Completer

The Agent's abstraction for LLM communication. The Completer is created from
an Anthropic client provided by the consuming application and acts as a
stateless Adapter: it receives a complete request (messages, tools, model
configuration), bridges to the SDK, and returns a streaming completion.

### Compaction

The process of summarizing or truncating conversation history to keep the
context within the LLM's token limits while preserving essential information.
A planned future capability of the Conversation State component.

### Context Window

The maximum number of tokens an LLM can process in a single request, including
both the conversation history and the new response. When the conversation
history exceeds the context window, the API returns an error and the consuming
application must apply a strategy such as compaction or truncation to continue.

### Agentic Loop

The inner runtime cycle within a single Agent run: send the conversation to
the LLM, receive a response, check if the response contains tool-use
requests, dispatch tools, append results, and repeat until the LLM produces
a final (non-tool-use) response. The agentic loop is driven by the Agent
and executes within a single call to `run`. Distinguished from the
conversation loop, which is the outer cycle driven by the user.

### Conversation Loop

The outer cycle of a multi-turn conversation: the user sends a message, the
Agent runs (executing the agentic loop) and produces a response, the user
sends the next message, and so on. The conversation loop is driven by the
consuming application, not by the library. Each iteration of the
conversation loop corresponds to one call to `run`.

### Conversation State

The library component that owns the message history for an agent
session. Stores user messages, assistant responses, and tool results, and
enforces correct message ordering and protocol conventions. The consuming
application can control resource-significant aspects such as history bounds
and compaction strategy.

### Extended Thinking

An Anthropic API feature that allows the model to perform chain-of-thought
reasoning in a dedicated thinking block before producing its visible response.
Useful for complex tasks requiring multi-step reasoning.

### Human-in-the-Loop (HITL)

A capability where individual tools are flagged as requiring human approval
before execution. When the LLM requests a call to a HITL-flagged tool, the
Tool Registry invokes an approval callback provided by the consuming
application. The callback receives the tool call details and returns a
binary approve/deny decision. On denial, an error result is sent back to
the LLM so it can adapt. The approval callback blocks the agentic loop
inline — `run` does not return mid-loop for HITL decisions.

### Approval Callback

The function registered with the Tool Registry that decides whether a
HITL-flagged Tool Call may proceed. Receives the tool name and arguments
and returns a binary approve/deny decision. Blocks the agentic loop
inline until the decision is made. See Human-in-the-Loop (HITL).

### Memory Tool

A tool that gives an agent access to persistent knowledge across conversations —
e.g., facts learned in prior sessions, user preferences, or accumulated context.
Not currently in scope; identified as a future capability that would enable
agent reuse across conversations.

### Progressive Capability

A design approach where an Agent starts with minimal functionality (simple
completion) and capabilities are layered on incrementally (tool use, HITL,
extended thinking, deterministic logic) rather than requiring all-or-nothing
configuration.

### Sub-Agent

An agent that is started by another agent as part of its workflow. A sub-agent
is a full agentic loop in its own right — with its own conversation state and
potentially its own tools — but is initiated and managed by a parent agent.
Sub-agent composition is a form of tool use from the parent's perspective, but
represents an independent agentic workflow.

### Tool Registry

The library component that manages the set of tools available to an
agent. The consuming application registers tool definitions and implementations
with the registry; the Agent uses it to provide tool definitions to the LLM and
to dispatch tool-use requests to the correct implementation.

### Tool Dispatch

The mechanism by which the harness routes a tool-use request from the LLM
to the appropriate tool implementation, executes it, and returns the result
to the conversation.

### Transient Error

An API error that is temporary and may succeed on retry — specifically rate
limit responses, network timeouts, and server errors. Distinguished from
non-transient errors (authentication failures, invalid requests) which
indicate a problem that retrying will not resolve.

### Vendor API

The HTTP API provided by an LLM provider (e.g., Anthropic, OpenAI) through
which the agent sends prompts and receives completions. Each vendor has its
own protocol, authentication, and tool-use conventions.

### OpenTelemetry (OTEL)

An open standard for distributed tracing, metrics, and logging. The library
uses the OTEL Trace API to create spans that represent units of work. The
OTEL API is a lightweight dependency that is a no-op when no SDK is
configured, allowing the consuming application to control whether and how
traces are collected.

### Span

A named, timed unit of work within a trace. Spans form parent-child trees
that represent the causal structure of an operation. In this library, spans
represent operations such as an agent run, an LLM call, or a tool execution.

### Structured Logging

Logging where each log entry consists of a message plus typed key-value
attributes, rather than a formatted string. The Go standard library provides
this via the `log/slog` package. Structured logs are machine-parseable and
integrate naturally with observability platforms.

### Trace

A tree of spans representing the complete path of an operation through the
system. A single agent run produces a trace with spans for the overall run,
each LLM call, and each tool execution.

<!-- ELICITATION GUIDANCE: During requirements gathering, watch for:
     - Terms that different stakeholders use differently
     - Terms that seem obvious but have subtle domain-specific meaning
     - Abbreviations and acronyms
     - Terms borrowed from other domains that might confuse
     Add new terms as they arise in any requirements discussion. -->
