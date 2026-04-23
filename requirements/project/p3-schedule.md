# P3: Schedule and Milestones

## Key Milestones

| Milestone | Status | Description | Depends On |
|-----------|--------|-------------|------------|
| M1: Basic Conversation | Complete | Agent can perform simple LLM completions with streaming via the Completer. Conversation state is managed by the library. Tracing spans and structured logs are emitted for the agent run and LLM calls. | — |
| M2: Tool Use | Complete | Agent supports tool registration and the full agentic loop (tool dispatch, result handling, loop termination). Tracing spans and structured logs cover tool dispatch and execution. | M1 |
| M3: HITL Example | Planned | Example application demonstrating tool-level human approval (S2.8) end-to-end — exercising both approval and denial paths against the live Anthropic API. The library capability shipped with M2; M3 delivers the working demonstration. | M2 |
| M4: Extended Thinking | Planned | Agent supports Anthropic's extended thinking feature. | M1 |
| M5: Deterministic Logic | Planned | Agent can incorporate non-LLM logic steps in workflows. | M2 |
| M6: Example Application | Planned | Dog-food application demonstrating the library's capabilities. | M2 (at minimum) |

## Iteration Plan

This is an open-ended personal/learning project with no fixed timeline.
Milestones are ordered by dependency and increasing sophistication rather
than by calendar dates. Development follows an iterative approach:

- **Early iterations (M1–M2):** Focus on core abstractions, API design, and
  the fundamental agentic loop. Observability (tracing and logging)
  is instrumented alongside each component as it is built. These establish
  the foundation that all later capabilities build on.
- **Middle iterations (M3–M5):** Demonstrate HITL via a dedicated example
  (M3 — library capability already shipped with M2) and add the remaining
  progressive capabilities (M4, M5). Each can be developed somewhat
  independently once M2 is complete.
- **Ongoing:** The example application (M6) should be started as early as M2
  to validate the library's API through real use.

## Constraints on Schedule

No hard deadlines. The pace is driven by available personal time and learning
goals rather than external commitments.
