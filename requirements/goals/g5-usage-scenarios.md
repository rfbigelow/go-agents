# G5: High-Level Usage Scenarios

## G5.1: Simple Chat Loop

**Actor:** Library consumer (G7.2)

**Goal:** Verify the library is working by running a basic conversation with the
LLM, with no tools or advanced features.

**Steps:**

1. The developer creates an Agent with a Completer and a system prompt.
2. The developer sends a user message to the Agent.
3. The Agent streams the LLM's response back to the developer's application.
4. The developer sends follow-up messages; the Agent maintains the conversation
   history across turns.

## G5.2: Tool-Using Agent

**Actor:** Library consumer (G7.2)

**Goal:** Build an agent that can take actions by calling tools as part of an
autonomous workflow.

**Steps:**

1. The developer creates an Agent and registers one or more tools.
2. The developer sends a user message describing a task.
3. The Agent enters the agentic loop: it sends the conversation to the LLM,
   the LLM requests tool calls, the Agent dispatches them, and repeats until the
   LLM produces a final response.
4. The developer receives the final response with the task completed.

## G5.3: Adding Extended Thinking

**Actor:** Library consumer (G7.2)

**Goal:** Enable extended thinking so the agent reasons through complex problems
before responding.

**Steps:**

1. The developer has a working agent (from G5.1 or G5.2).
2. The developer configures extended thinking on the Agent — choosing a
   thinking mode (`adaptive` to let the model decide whether and how much to
   think, or `enabled` with an explicit `budget_tokens` cap for manual
   control).
3. By default, the library suppresses thinking text in the response stream
   (`display: "omitted"`); the developer opts in to `display: "summarized"` if
   they want to surface a summary of the model's reasoning to end users.
4. During the run, the model emits thinking blocks; the library forwards
   incremental thinking text via the event stream (when `summarized`) and
   preserves each block's encrypted signature in conversation state so that
   multi-turn tool-use loops continue to satisfy the API's protocol
   requirements.
5. The developer's application receives the final response as before; thinking
   blocks and signatures are managed transparently by the library.

## G5.4: Composing Agents with Sub-Agents

**Actor:** Library consumer (G7.2)

**Goal:** Build an agent that delegates parts of its workflow to specialized
sub-agents.

**Steps:**

1. The developer creates a parent agent with tools.
2. One of the parent's tools creates and runs a sub-agent — a separate agentic
   loop with its own conversation state and potentially its own tools.
3. The parent agent calls this tool during its workflow; the sub-agent runs to
   completion and returns its result.
4. The parent agent continues its workflow using the sub-agent's result.

## G5.6: Agent with Tool-Level Human Approval

**Actor:** Library consumer (G7.2)

**Goal:** Build an agent where certain tools — typically destructive,
irreversible, or privileged — require human approval before the LLM is allowed
to execute them.

**Steps:**

1. The developer creates an Agent and registers tools, flagging one or more as
   requiring human approval. The developer registers an approval callback with
   the Tool Registry.
2. The developer sends a user message describing a task that may require a
   flagged tool.
3. When the LLM requests a flagged tool during the agentic loop, the approval
   callback is invoked with the tool name and arguments. The developer's
   application surfaces the request to a human, who decides approve or deny.
4. On approval, the tool executes and the loop continues. On denial, the LLM
   receives a denial result and adapts — trying a different approach, asking
   for clarification, or producing a final response.
5. The developer receives the final response.

## G5.7: Tuning Output Effort

**Actor:** Library consumer (G7.2)

**Goal:** Trade off response thoroughness against latency and token cost using
the Anthropic effort parameter.

**Steps:**

1. The developer has a working agent (from G5.1 or G5.2).
2. The developer sets an effort level on the Agent (e.g., `low` for
   high-volume or latency-sensitive workloads, `high` for the API's default
   thoroughness, `max` for the highest capability on supported models).
3. The model adjusts its overall token allocation accordingly — affecting
   text length, the number and verbosity of tool calls, and (when extended
   thinking is configured) the depth of reasoning. Effort applies whether or
   not extended thinking is enabled.
4. The developer's application receives the response through the same stream
   and conversation interfaces as before; effort changes the shape of the
   response without changing the library's surface.

## G5.5: Agent Reuse Across Conversations (Future)

**Actor:** Library consumer (G7.2)

**Goal:** Reuse an agent across multiple conversations, retaining knowledge from
prior sessions.

**Steps:**

1. The developer creates an agent with access to a memory tool.
2. During a conversation, the agent stores relevant facts to memory.
3. In a later conversation, the agent retrieves prior knowledge from memory to
   inform its responses.

*Note: This scenario is not in current scope. It is included to capture the
anticipated direction and to inform architectural decisions (see G6).*
