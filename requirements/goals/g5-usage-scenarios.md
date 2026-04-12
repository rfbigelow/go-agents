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
2. The developer enables extended thinking on the Agent.
3. The Agent includes thinking blocks in its LLM interactions; the model reasons
   through the problem before producing its visible response.
4. The developer's application receives the response as before — the thinking
   is handled transparently by the library.

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
