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

**Purpose:** The Agent's abstraction for LLM communication. The Completer is
created from an Anthropic client provided by the consuming application and
acts as a stateless Adapter — it receives a complete request (messages, tools,
model configuration), bridges to the SDK, and returns a streaming response.
**Interacts with:** Agent (as its LLM communication abstraction), Anthropic Go
SDK (E2.2).
**Key properties:** Stateless — holds a reference to the consumer-provided
Anthropic client but maintains no state of its own. Delegates to the SDK,
including the SDK's built-in transient error retry behavior.

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
