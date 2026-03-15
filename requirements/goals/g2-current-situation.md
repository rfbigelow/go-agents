# G2: Current Situation

## Current Processes

Agent projects are built directly against vendor LLM APIs (e.g., Anthropic,
OpenAI). Each project implements its own conversation loop, tool handling,
error management, and other agent infrastructure from scratch.

## Pain Points

- **Repeated work.** Common agent patterns (tool dispatch, conversation
  management, etc.) are re-implemented for every new project.
- **No accumulated learning.** Lessons learned about effective agent patterns
  in one project don't transfer structurally to the next — they exist only
  as developer knowledge, not as reusable code.
- **Inconsistency.** Each project may implement the same patterns differently,
  making it harder to compare approaches or share improvements.
