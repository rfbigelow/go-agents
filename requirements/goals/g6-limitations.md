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
