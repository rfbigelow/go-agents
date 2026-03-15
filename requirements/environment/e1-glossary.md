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

### Progressive Capability

A design approach where an Agent starts with minimal functionality (simple
completion) and capabilities are layered on incrementally (tool use, HITL,
extended thinking, deterministic logic) rather than requiring all-or-nothing
configuration.

### Tool Dispatch

The mechanism by which the harness routes a tool-use request from the LLM
to the appropriate tool implementation, executes it, and returns the result
to the conversation.

### Vendor API

The HTTP API provided by an LLM provider (e.g., Anthropic, OpenAI) through
which the agent sends prompts and receives completions. Each vendor has its
own protocol, authentication, and tool-use conventions.

<!-- ELICITATION GUIDANCE: During requirements gathering, watch for:
     - Terms that different stakeholders use differently
     - Terms that seem obvious but have subtle domain-specific meaning
     - Abbreviations and acronyms
     - Terms borrowed from other domains that might confuse
     Add new terms as they arise in any requirements discussion. -->
