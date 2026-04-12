# E5: Effects

### E5.1: Consumer Applications Depend on go-agents

**What changes:** Agent applications that adopt the library take on a dependency
on go-agents and, transitively, on the Anthropic Go SDK (E2.2).
**How it changes:** Instead of each application directly depending on and
interacting with the Anthropic Go SDK, applications depend on go-agents, which
mediates SDK access. The library's API stability and release cadence become a
factor in consumer application maintenance.
**Who is affected:** Agent application developers who consume the library.

### E5.2: Agentic Loop Management Is Delegated

**What changes:** Agent applications no longer implement their own agentic
loops.
**How it changes:** The library manages the agentic loop — sending messages
to the LLM, processing tool-use requests, executing tools, and determining
when a response is ready. The application retains control of obtaining user input
and initiating conversations; the library handles the loop internals.
**Who is affected:** Agent application developers who consume the library.
