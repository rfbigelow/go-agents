# System Overview

## System Architecture

The library provides four components: an **Agent** (S1.1) that drives the
agentic conversation loop, a **Client** (S1.2) that wraps the Anthropic Go SDK
for streaming LLM communication, a **Tool Registry** (S1.3) for registering and
dispatching tools, and managed **Conversation State** (S1.4). See S1 for details.

## Key Functionality

The Agent manages the conversation loop — sending messages to the LLM,
dispatching tool calls in parallel, and repeating until a final response is
ready. Capabilities are progressive: a minimal Agent does simple completions;
tool use, HITL, extended thinking, deterministic logic, and sub-agent composition
are layered on incrementally. Errors and panics in tools are isolated and
reported back to the LLM. A configurable iteration limit guards against runaway
loops. All operations are instrumented with distributed traces (via OTEL) and
structured logs (via slog), giving consumers full visibility into agent behavior
without the library imposing any observability infrastructure. See S2 for
details.

## External Interfaces

The library exposes a Go API for agent creation, configuration, and execution
(S3). The consuming application controls the interaction flow — the library
never takes over the main loop (E6.1). Externally, the library communicates
with the Anthropic Messages API via the Go SDK (E2.1, E2.2). See S3 for
details.

## Chapter Index

| Chapter | Contents |
|---------|----------|
| [s1](s1-components.md) | Major system components and their relationships |
| [s2](s2-functionality.md) | What the system does — operations and behaviors |
| [s3](s3-interfaces.md) | External interfaces: APIs, UI, integrations |
| [s4](s4-scenarios.md) | Detailed usage scenarios including edge cases |
| [s5](s5-priorities.md) | Relative importance and ordering of requirements |
| [s6](s6-verification.md) | How to verify the system meets its requirements |
