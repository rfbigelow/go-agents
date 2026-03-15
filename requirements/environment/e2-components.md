# E2: Environment Components

## E2.1: Anthropic Messages API

**Type:** API
**Description:** The LLM completion API provided by Anthropic. Supports
multi-turn conversation, tool use, and streaming. Accessed via the Anthropic
Go SDK.
**Interaction:** The library sends conversation messages (including tool
results) and receives model completions (including tool-use requests).
**Owner:** Anthropic. API changes are versioned but could affect the library
through SDK updates.

## E2.2: Anthropic Go SDK

**Type:** Library dependency
**Description:** Anthropic's official Go client library for the Messages API.
Handles authentication, request construction, and response parsing.
**Interaction:** The library depends on the SDK as its primary external
dependency. Agent applications interact with Anthropic through the SDK via
the library's abstractions.
**Owner:** Anthropic.
