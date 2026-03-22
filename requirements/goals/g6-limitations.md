# G6: Limitations and Exclusions

## Exclusions

### G6.1: No Multi-Provider Support

This library supports Anthropic as the sole LLM provider. There is no
abstraction layer for swapping providers, and no plan to support OpenAI,
Google, or other LLM APIs. The library may use Anthropic-specific features
directly without concern for portability across providers.

### G6.2: Agent Applications Only

This library is purpose-built for agentic workflows — applications where an
LLM drives multi-step, tool-using behavior. It is not a general-purpose LLM
client library, a chatbot framework, or a toolkit for non-agent use cases
such as batch embedding, fine-tuning, or content generation pipelines.

### G6.3: No Sub-Agent Nesting

Sub-agents cannot spawn further sub-agents. The maximum agent nesting depth is
one: a parent agent may launch sub-agents, but those sub-agents may not
themselves launch sub-agents.

### G6.4: No Cross-Conversation Memory (Current Scope)

The library does not currently support persisting agent knowledge across
conversations. Agent reuse across conversations would require a memory mechanism
(see G5.5) that is not in scope for the initial release.
