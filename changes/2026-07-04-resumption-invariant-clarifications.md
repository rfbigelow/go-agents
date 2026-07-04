# Change Request: Resumption Invariant Clarifications

**Date:** 2026-07-04
**Status:** Accepted

## Ask

While implementing S2.15 (Conversation Resumption, M9), two of the five
validation invariants turned out to admit histories the Anthropic Messages
API rejects. Tighten the wording so the invariants enforce the protocol they
exist to protect:

1. Require the history to start with a user message.
2. Require a `tool_result` block's ID to match a `tool_use` in the
   *immediately preceding* assistant message, not merely any preceding
   message.

## Analysis

Rule 1 constrains only the end of the history and rule 2 requires only
alternation, so a literal implementation would accept `[assistant]` or
`[assistant, user, assistant]`. The Messages API requires the first message
to use the `user` role, so such a history validates at construction and then
fails on the first `run` — the opposite of the constructor-time error S2.15
promises. Every history produced by `messages` (S2.6) starts with a user
message, so the tightened rule rejects only application-constructed
histories that could never run.

Rule 4 said "no `tool_result` block appears without a preceding `tool_use`
for the same ID", without rule 3's "immediately following/preceding"
qualifier. A literal reading accepts a `tool_result` answering a `tool_use`
from several turns back, which the API rejects and which would duplicate a
result rule 3 already required in the intervening turn. The strict reading —
the ID must appear in the immediately preceding assistant message — is the
only interpretation consistent with the tool-use protocol, and is cheaper to
check (no global seen-ID set).

Alternatives considered: validating literally and letting the API report the
error on the next `run` (rejected — S2.15's stated purpose is constructor-time
validation with no partial state), and enforcing the strict readings in code
only, as documented interpretation (rejected — the spec is the source of
truth; silent divergence between spec wording and enforced behavior is the
failure mode the PEGS process exists to prevent).

## PEGS Impact

- **S2.15 (Conversation Resumption):** invariant 2 now reads "User and
  assistant messages alternate, starting with a user message." Invariant 4
  now reads "No `tool_result` block appears without a matching `tool_use`
  for the same ID in the immediately preceding assistant message."
- **S6.24 (Conversation Resumption verification):** malformed-history case
  (2) gains an assistant-first history; case (4) wording aligned with the
  tightened invariant 4.
- No other requirements change; S2.6-committed histories satisfy both
  tightened invariants by construction.
