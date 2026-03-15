# S1: System Components

## S1.1: Agent

**Purpose:** The central runtime that drives an agentic workflow. Manages the
conversation loop, coordinates tool execution, and handles interaction with
the LLM. Designed for progressive capability addition — a minimal Agent
performs simple completions; capabilities such as tool use, human-in-the-loop
interaction, extended thinking, and deterministic logic are layered on
incrementally.
**Interacts with:** Client (to communicate with the LLM), Tool registry (to
dispatch tool calls), conversation state.
**Key properties:** Must support the full agentic spectrum from simple
single-turn completion to autonomous multi-step workflows. Capabilities are
composable and opt-in.

## S1.2: Client

**Purpose:** Wraps the Anthropic Go SDK to provide the interface the Agent
uses to communicate with the LLM. Encapsulates API-specific concerns such
as request construction, streaming, and transient error handling.
**Interacts with:** Anthropic Go SDK (E2.2), Agent.
**Key properties:** Supports streaming responses. Handles transient API
errors (retries, rate limits) unless already handled by the SDK.

## S1.3: Tool Registry

**Purpose:** Manages the set of tools available to an Agent. Provides tool
definitions to the LLM (for tool-use protocol) and dispatches tool-use
requests to the appropriate implementation.
**Interacts with:** Agent, individual tool implementations provided by the
consuming application.
**Key properties:** Tools are defined by the consuming application, not the
library. The registry provides the dispatch mechanism and the interface
contract.

## S1.4: Conversation State

**Purpose:** Owns the message history for an agent session. Stores the
sequence of user messages, assistant responses, and tool results that
constitute the conversation context.
**Interacts with:** Agent.
**Key properties:** Library-managed, not consumer-managed. Must support
future extension for features like context compaction (summarizing or
truncating history to stay within token limits).

## Component Relationships

```
Consuming Application
    │
    ├── creates and configures ──▶ Agent
    │                                │
    │                                ├── uses ──▶ Client ──▶ Anthropic SDK ──▶ Anthropic API
    │                                │
    │                                ├── uses ──▶ Tool Registry
    │                                │               │
    │                                │               └── dispatches to ──▶ Tool Implementations
    │                                │                     (provided by consuming app)
    │                                │
    │                                └── owns ──▶ Conversation State
    │
    └── provides ──▶ Tool Implementations
```
