# Environment Overview

## Key External Systems and Entities

The library interacts with two external components owned by Anthropic:
the **Messages API** (E2.1) for LLM completions and the **Anthropic Go SDK**
(E2.2) which provides the Go client for that API. For observability, the
library depends on the **OpenTelemetry Trace API** (E2.3) for distributed
tracing and the Go standard library's **slog** package (E2.4) for structured
logging. See E2 for details.

## Critical Constraints

The library is constrained to **Go 1.25+** (E3.1), **minimal dependencies**
beyond stdlib, the Anthropic SDK, and the OTEL Trace API (E3.2), **platform
agnosticism** (E3.3), **MIT license compatibility** (E3.4), and **consumer
resource control** (E3.5). See E3 for details.

## Important Assumptions

We assume Anthropic is the **sole LLM provider** (E4.1), the project has a
**single developer** (E4.2), and the Anthropic Go SDK provides a **reasonably
stable API** (E4.3). See E4 for details.

## Effects and Invariants

Introducing the library has two main environmental effects: consumer applications
take on a **dependency** on go-agents and transitively on the SDK (E5.1), and
**agentic loop management is delegated** to the library (E5.2). Three
invariants must hold: the **application controls execution flow** (E6.1), all
API communication is **protocol-compliant** (E6.2), and the **consumer owns its
resources** (E6.3). See E5 and E6 for details.

## Chapter Index

| Chapter | Contents |
|---------|----------|
| [e1](e1-glossary.md) | Definitions of domain terms used in requirements |
| [e2](e2-components.md) | External entities the system interacts with |
| [e3](e3-constraints.md) | External limitations the system must respect |
| [e4](e4-assumptions.md) | Assumed properties of the environment |
| [e5](e5-effects.md) | Changes the system will cause in the environment |
| [e6](e6-invariants.md) | Environmental properties that must remain true |
