# S5: Priorities

## Priority Levels

| Priority | Meaning |
|----------|---------|
| **Must** | System is not viable without this. Required for first release. |
| **Should** | Important and expected, but system is usable without it. |
| **Could** | Desirable if time and resources permit. |
| **Won't (deferred)** | Explicitly deferred. May be addressed in a future iteration. |

## Requirements by Priority

### Must

These form the core library — an agent that can hold conversations, use tools,
and handle errors. Corresponds to milestones M1–M2 (P3).

- **S2.1** — Agent creation and configuration
- **S2.2** — Conversation loop execution
- **S2.3** — Streaming responses
- **S2.4** — Tool registration
- **S2.5** — Tool dispatch and execution
- **S2.6** — Conversation state management
- **S2.7** — Transient error handling

### Should

Progressive capabilities that make the library significantly more useful but
are not required for a viable first release. Corresponds to milestones M3–M5
(P3).

- **S2.8** — Human-in-the-loop
- **S2.9** — Extended thinking
- **S2.10** — Deterministic logic
- **S2.11** — Sub-agent composition

### Could

No requirements currently at this level.

### Deferred

- **Memory tool** — Persistent knowledge across conversations (G5.5). Explicitly
  out of initial scope per G6.4. Deferred until core capabilities are stable and
  a real use case drives the design.

## Ordering Constraints

- S2.2 (conversation loop) depends on S2.1 (agent creation), S2.6
  (conversation state), and S2.3 (streaming).
- S2.5 (tool dispatch) depends on S2.4 (tool registration).
- S2.2 depends on S2.5 for tool-using agents, but a no-tool agent can run
  without it.
- S2.8 (HITL), S2.9 (extended thinking), and S2.10 (deterministic logic) each
  depend on S2.2 but are independent of each other.
- S2.11 (sub-agent composition) depends on S2.2 and S2.5 (a sub-agent is
  invoked as a tool).
