# S1: System Components

## S1.1: Agent

**Purpose:** The central runtime that drives an agentic workflow. Manages the
conversation loop, coordinates tool execution, and handles interaction with
the LLM. Designed for progressive capability addition — a minimal Agent
performs simple completions; capabilities such as tool use, human-in-the-loop
interaction, extended thinking, and deterministic logic are layered on
incrementally.
**Interacts with:** Completer (to communicate with the LLM), Tool Registry (to
dispatch tool calls), Conversation State.
**Key properties:** Must support the full agentic spectrum from simple
single-turn completion to autonomous multi-step workflows. Capabilities are
composable and opt-in.

## S1.2: Completer

**Purpose:** A Go interface that the Agent uses to get completions from the
LLM. The interface abstracts LLM communication so the Agent depends only on
the Completer contract, not on any specific SDK or client. The library provides
a default implementation that wraps an Anthropic client created and owned by
the consuming application.
**Interacts with:** Agent (as its LLM communication interface), Anthropic Go
SDK (E2.2) via the library-provided implementation.
**Key properties:** Defined as a Go interface — consumers can provide
alternative implementations. The library-provided implementation supports
streaming responses and handles transient API errors (retries, rate limits).

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
**Key properties:** Library-provided, consumer-controllable. The library manages
conversation state by default but the consumer can influence resource-significant
decisions such as history bounds and compaction strategy (per E3.5). Must support
features like sliding context windows and context compaction.

## Component Relationships

```
Consuming Application
    │
    ├── creates ──▶ Anthropic Client (from SDK)
    │                     │
    │                     └── wrapped by ──▶ Completer (library-provided impl)
    │                                            │
    ├── creates and configures ──▶ Agent ────uses─┘
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
