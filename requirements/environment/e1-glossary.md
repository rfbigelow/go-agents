# E1: Glossary

## Terms

### Agent

A software system in which an LLM autonomously drives a multi-step workflow,
making decisions about which actions to take based on context and intermediate
results. Distinguished from a simple LLM chat interaction by its ability to
use tools and pursue goals across multiple turns.

### Harness

The reusable runtime infrastructure that manages an agent's execution: the
conversation loop, tool dispatch, error handling, and interaction with the
LLM API. The harness is what this library provides; agent-specific behavior
is layered on top of it.

### Tool

A function or capability made available to the agent (via the LLM's tool-use
protocol) that allows it to take actions beyond generating text — e.g.,
reading files, making API calls, or executing commands.

### Agentic Spectrum

The range of agent sophistication, from simple single-turn LLM completions
(no tools, no loops) through tool-using agents to fully autonomous multi-step
workflows with human-in-the-loop control. The library is designed to support
applications at any point on this spectrum.

### Compaction

The process of summarizing or truncating conversation history to keep the
context within the LLM's token limits while preserving essential information.
A planned future capability of the Conversation State component.

### Conversation Loop

The core runtime cycle of an agent: send messages to the LLM, receive a
response, check if the response contains tool-use requests, execute tools,
append results, and repeat until the LLM produces a final (non-tool-use)
response. Also called an "agentic loop."

### Extended Thinking

An Anthropic API feature that allows the model to perform chain-of-thought
reasoning in a dedicated thinking block before producing its visible response.
Useful for complex tasks requiring multi-step reasoning.

### Human-in-the-Loop (HITL)

A workflow pattern where the agent pauses execution to request input,
approval, or correction from a human before continuing. Enables human
oversight of autonomous agent behavior.

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

### Tool Dispatch

The mechanism by which the harness routes a tool-use request from the LLM
to the appropriate tool implementation, executes it, and returns the result
to the conversation.

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
