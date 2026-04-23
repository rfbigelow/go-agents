# S4: Detailed Usage Scenarios

## S4.1: Simple Conversation with Error Recovery

**Elaborates:** G5.1
**Preconditions:** Agent created with a Completer and system prompt. No tools
registered.
**Actor:** Library consumer (G7.2)

**Main flow:**

1. The consumer sends a user message to the Agent (S2.2).
2. The Agent sends the message and conversation history to the LLM via the
   Completer (S1.2).
3. The Completer streams the response back (S2.3).
4. The Agent appends the user message and assistant response to Conversation
   State (S1.4) and returns the response to the consumer.
5. The consumer sends follow-up messages; conversation history grows across
   turns.

**Alternate flows:**

- **Context window exceeded:** The conversation history plus the new message
  exceeds the model's context window. The API returns an error. The Agent
  returns the error to the consumer without appending the new message to
  history. The consumer can apply a conversation management strategy (clear,
  compaction) and retry.

**Error cases:**

- **API error on any turn:** The Completer receives a non-transient error from
  the Anthropic API (e.g., invalid request, authentication failure). The Agent
  returns the error to the consumer. Conversation history is preserved as it
  was before the failing message — the message that prompted the error is not
  appended.
- **Transient API error:** The Completer receives a transient error (rate limit,
  timeout, server error). Retry behavior per S2.7 applies. If retries are
  exhausted, the error propagates to the consumer as above.

## S4.2: Tool Execution — Errors, Panics, and Loop Limits

**Elaborates:** G5.2
**Preconditions:** Agent created with one or more tools registered (S2.4).
**Actor:** Library consumer (G7.2)

**Main flow:**

1. The consumer sends a user message describing a task.
2. The Agent enters the agentic loop (S2.2). The LLM requests one or more
   tool calls.
3. The Agent dispatches all tool calls in parallel via the Tool Registry (S2.5).
4. Tool results are appended to conversation state and sent to the LLM in the
   next turn.
5. The loop repeats until the LLM produces a final response with no tool-use
   requests.

**Alternate flows:**

- **Unknown tool name:** The LLM requests a tool that is not in the registry.
  The Agent sends an error tool result back to the LLM describing the unknown
  tool. The agentic loop continues — the LLM can adjust.
- **Maximum iterations reached:** The agentic loop hits the configured
  maximum iteration count. The Agent stops the loop and returns an error to
  the consumer. Conversation state is preserved up to the last completed turn.

**Error cases:**

- **Tool returns an error:** The tool implementation returns a Go error. The
  Agent converts it to an error tool result and sends it back to the LLM. The
  agentic loop continues.
- **Tool panics:** The Agent recovers the panic via `recover()`, converts it
  to an error tool result, and sends it back to the LLM. The agentic loop
  continues. The panic does not propagate to the consumer or affect other
  concurrent tool calls.
- **Multiple tool failures in one turn:** Each tool call is independent. Failures
  (errors or panics) in individual tools produce individual error results. The
  LLM receives a mix of successful and error results and decides how to proceed.

## S4.3: Sub-Agent Failure Isolation

**Elaborates:** G5.4
**Preconditions:** Parent agent created with a tool that spawns a sub-agent
(S2.11).
**Actor:** Library consumer (G7.2)

**Main flow:**

1. The parent agent's agentic loop calls a tool that creates and runs a
   sub-agent.
2. The sub-agent runs its own agentic loop to completion.
3. The sub-agent's result is returned as a tool result to the parent.
4. The parent continues its workflow.

**Alternate flows:**

- **Parallel sub-agents:** The LLM requests multiple tool calls in one turn,
  some of which spawn sub-agents. All run in parallel. Each sub-agent has its
  own conversation state and response stream (S2.11).

**Error cases:**

- **Sub-agent returns an error:** The sub-agent's run fails (API error, max
  iterations, etc.). The error is converted to an error tool result for the
  parent agent. The parent's agentic loop continues — the parent can
  react to the failure.
- **Sub-agent tool panics:** The parent agent recovers the panic (same as
  S4.2), converts it to an error tool result, and continues.
- **Sub-agent attempts to nest:** A sub-agent's tool attempts to spawn another
  sub-agent. This is rejected per the one-level nesting limit (S2.11, G6.3).
  The tool receives an error result.

## S4.4: Extended Thinking — Transparent Reasoning

**Elaborates:** G5.3
**Preconditions:** Agent created with extended thinking enabled (S2.9).
**Actor:** Library consumer (G7.2)

**Main flow:**

1. The consumer sends a message requiring complex reasoning.
2. The Agent includes the extended thinking configuration in the API request.
3. The LLM produces thinking blocks followed by its visible response.
4. The Agent streams the visible response to the consumer. Thinking blocks are
   handled transparently by the library.

**Alternate flows:**

- **Thinking with tool use:** The LLM reasons in a thinking block, then
  requests tool calls. The agentic loop proceeds normally (S4.2). Thinking
  blocks may appear before any turn's tool-call requests.

## S4.5: Tool Approval — Approval, Denial, and Mixed Turns

**Elaborates:** G5.6
**Preconditions:** Agent created with one or more HITL-flagged tools registered
(S2.4) and an approval callback registered on the Tool Registry (S2.8).
**Actor:** Library consumer (G7.2)

**Main flow:**

1. The consumer sends a user message describing a task.
2. The Agent enters the agentic loop (S2.2). The LLM responds with a tool-use
   request for a HITL-flagged tool.
3. The Tool Registry invokes the approval callback with the tool name and
   arguments. The callback returns approve.
4. The tool executes normally (S2.5). Its result is appended to conversation
   state and sent to the LLM in the next turn.
5. The loop continues until the LLM produces a final response.

**Alternate flows:**

- **Denial:** The approval callback returns deny. The Tool Registry produces
  an error tool result indicating user denial; the tool does not execute. The
  LLM receives the denial and can adapt (try another approach, ask for
  clarification, or produce a final response). The agentic loop continues.
- **Mixed HITL and non-HITL tools in one turn:** The LLM requests multiple
  tool calls in a single response, some flagged for approval and some not.
  The Tool Registry invokes each HITL callback serially, in the order the
  tool calls appear in the LLM response. After all approval decisions are
  made, approved tools and non-HITL tools execute in parallel. Denied tools
  receive error results without executing.
- **All HITL tools in a turn denied:** The LLM receives only denial results
  and either adapts in the next turn or produces a final response without
  further tool calls.

**Error cases:**

- **Approval callback panics:** The Agent does not recover the panic. The
  panic propagates as a fatal error from `run`. Conversation state is
  preserved up to the last completed turn — the partial turn that invoked
  the callback is not retained. Rationale: a broken approval gate is a
  safety issue, not a recoverable tool-execution issue.
- **Missing approval callback:** A setup-time error, not a runtime case.
  Registering a HITL-flagged tool without an approval callback on the Tool
  Registry fails at registration (S2.4, S6.21).
