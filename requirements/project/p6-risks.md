# P6: Risk and Mitigation Analysis

## Risks

### P6.1: Anthropic SDK Instability

**Likelihood:** Low
**Impact:** Medium
**Description:** The Anthropic Go SDK may introduce breaking changes that
require library updates. The library depends on SDK types throughout its API
(E2.2, E4.3).
**Affected areas:** E4.3, S1.2, S2.7
**Mitigation:** Pin to a specific SDK version. Track SDK releases. The Completer
Completer (S1.2) limits the SDK surface area exposed to consumers.
**Contingency:** If breakage is frequent, introduce an anti-corruption layer
between the library and the SDK.

### P6.2: API Design Lock-In

**Likelihood:** Medium
**Impact:** High
**Description:** The library's public Go API, once consumed by agent
applications, is difficult to change without breaking consumers. Getting the
API wrong in early milestones (M1–M2) means painful refactoring later.
**Affected areas:** S2, S3
**Mitigation:** Start with the example application (M6) early — at M2 — to
validate the API through real use before the surface area grows. Keep the
public API minimal; prefer fewer exported types and functions.
**Contingency:** If a breaking API change is needed, use a major version bump
per Go module conventions.

### P6.3: Scope Creep Toward Framework

**Likelihood:** Medium
**Impact:** Medium
**Description:** The library could drift from "reusable agent patterns" toward
a prescriptive framework that constrains how consumers structure their
applications. This would contradict E6.1 (application controls execution flow)
and G6.2 (not a general-purpose LLM client).
**Affected areas:** G6, E6.1, S2
**Mitigation:** Use the scope limitations (G6) and invariants (E6) as
guardrails when evaluating new features. Each addition should pass the test:
"does this help the consumer build agents, or does it take control away from
them?"
**Contingency:** Remove or extract features that violate the library boundary.

### P6.4: HITL Execution Model Complexity

**Likelihood:** Medium
**Impact:** Medium
**Description:** The human-in-the-loop execution model (S2.8) is still
undefined. It could complicate the core Agent API if it requires a fundamentally
different return type or control flow from normal conversation loop execution.
**Affected areas:** S2.2, S2.8, S3
**Mitigation:** Design HITL to reuse the existing execution model — the Agent
returns a tagged response indicating whether it is a final answer or a request
for human input (see TODO in S2.8). This avoids a separate execution path.
**Contingency:** If the simple tagged-response model proves insufficient,
revisit before implementing M3 rather than forcing a complex design.

### P6.5: No External Stakeholders

**Likelihood:** High
**Impact:** Medium
**Description:** The sole developer is also the sole consumer (E4.2, G7). With
no external users or reviewers, there is no feedback loop to catch API usability
issues, missing capabilities, or assumptions that are invisible to someone who
is both builder and user.
**Affected areas:** G7, S3, P6.2
**Mitigation:** The example application (M6) provides some distance between
"library developer" and "library consumer" perspectives. AI-assisted
requirements elicitation and review can challenge assumptions. Writing
requirements before code forces explicit articulation of decisions.
**Contingency:** If the library is later opened to external consumers, plan
for an API review and usability feedback round before committing to stability.
