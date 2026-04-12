# G4: Functionality Overview

go-agents is a lightweight agent harness that allows developers to write agents
in Go while focusing on their application logic.

## Core Functions

**G4.1: Run agentic conversations.** The developer provides a prompt and the
library manages the back-and-forth with the LLM — including multi-turn tool
use — until the LLM produces a final response. The developer does not need to
manage the agentic loop themselves.

**G4.2: Stream responses.** The library delivers LLM output incrementally as it
is generated, so applications can display or process partial results without
waiting for the complete response.

**G4.3: Register and dispatch tools.** The developer defines tools — their
names, descriptions, input schemas, and implementations — and the library
handles presenting them to the LLM and dispatching calls when the LLM requests
them. The developer focuses on what each tool does, not on the tool-use
protocol.

**G4.4: Manage conversation history.** The library tracks the full conversation
(user messages, assistant responses, tool results), enforcing correct message
ordering and protocol conventions. The developer controls resource-significant
aspects such as history bounds and compaction strategy (see E3.5).

**G4.5: Handle API errors gracefully.** Transient failures (rate limits,
timeouts, server errors) are retried automatically. The developer only needs
to handle permanent errors.

**G4.6: Add capabilities progressively.** A minimal agent performs simple chat
completions. The developer layers on capabilities — tool use, human-in-the-loop
interaction, extended thinking, deterministic logic — incrementally as needed.
There is no upfront complexity tax for capabilities the agent does not use.

**G4.7: Provide observability.** The library instruments its operations with
distributed traces and structured logs. Traces form a span tree covering the
agent run, each LLM call, each tool execution, and sub-agent invocations,
enabling the developer to understand timing, causality, and errors across
the full agent workflow. Structured logs emit key lifecycle events with
contextual attributes. The developer controls trace collection (via OTEL SDK
configuration) and log output (via slog handler configuration) — the library
provides the instrumentation, not the infrastructure.

## How Users Will Interact With the System

go-agents is a Go library. Developers import it into their applications, create
and configure an Agent, register tools, and run conversations through the
library's API. There is no CLI, UI, or standalone service — the library exists
entirely within the developer's application process.
