# P3: Schedule and Milestones

## Key Milestones

| Milestone | Status | Description | Depends On |
|-----------|--------|-------------|------------|
| M1: Basic Conversation | Complete | Agent can perform simple LLM completions with streaming via the Completer. Conversation state is managed by the library. Tracing spans and structured logs are emitted for the agent run and LLM calls. | — |
| M2: Tool Use | Complete | Agent supports tool registration and the full agent loop (tool dispatch, result handling, loop termination). Tracing spans and structured logs cover tool dispatch and execution. | M1 |
| M3: HITL Example | Complete | Example application demonstrating tool-level human approval (S2.8) end-to-end — exercising both approval and denial paths against the live Anthropic API. The library capability shipped with M2; M3 delivered the working demonstration in `examples/hitl/`. | M2 |
| M4: Extended Thinking | Complete | Agent exposes Anthropic's Extended Thinking (S2.9) and the related `output_config.effort` parameter (S2.16) on Agent configuration. Streaming surfaces `thinking_delta` and `signature_delta` events; thinking blocks (with signatures) are preserved across multi-turn tool-use loops via `Message.ToParam()`. Demonstrated in `examples/chat/` with per-model thinking-mode selection (adaptive / enabled / off) and gray-text rendering of thinking. | M1 |
| M5: Deterministic Logic | Complete | Agent supports typed loop hooks at `PreLLMCall`, `PreToolUse`, and `PostToolUse` (S2.10) for interposing deterministic non-LLM logic — validation, transformation, substitution, and abort — on the agent loop. Configured via `with_hooks` on the Agent ADT. | M2 |
| M6: Example Application | Planned | Dog-food application demonstrating the library's capabilities. | M2 (at minimum) |
| M7: Sub-Agent Composition | Complete | A tool can create and run a sub-agent — a separate agent loop with its own conversation state, tools, and response stream (S2.11). Sub-agents run as tool calls (one-shot or multi-turn via session handle), support stream forwarding with attribution, propagate HITL approval to the parent, and are limited to one level of nesting. Demonstrated in `examples/sub-agent/`. | M2 |
| M8: Prompt Caching | Complete | The Agent places cache-control breakpoints on the system prompt, tool definitions, and conversation history (S2.17) so the Anthropic API can cache and reuse stable prefixes. Enabled by default with opt-out; cache token metrics are surfaced through tracing (S2.12) and logging (S2.13). | M2 |

## Iteration Plan

This is an open-ended personal/learning project with no fixed timeline.
Milestones are ordered by dependency and increasing sophistication rather
than by calendar dates. Development follows an iterative approach:

- **Early iterations (M1–M2):** Focus on core abstractions, API design, and
  the fundamental agent loop. Observability (tracing and logging)
  is instrumented alongside each component as it is built. These establish
  the foundation that all later capabilities build on.
- **Middle iterations (M3–M5, M7–M8):** Demonstrate HITL via a dedicated
  example (M3 — library capability already shipped with M2) and add the
  remaining progressive capabilities — extended thinking (M4), deterministic
  logic (M5), sub-agent composition (M7), and prompt caching (M8). Each can be
  developed somewhat independently once M2 is complete.
- **Ongoing:** The example application (M6) should be started as early as M2
  to validate the library's API through real use. It is numbered M6 but
  remains in progress while the higher-numbered capability milestones (M7,
  M8) have shipped — milestones are ordered by dependency and sophistication,
  not by completion date.

## Constraints on Schedule

No hard deadlines. The pace is driven by available personal time and learning
goals rather than external commitments.
