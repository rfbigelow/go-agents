# Change Request: Sub-Agent Shared-Gate Approval Panic Propagation

**Date:** 2026-07-03
**Status:** Accepted

## Ask

Issue #10: S2.8/S2.11 do not specify parent behavior when a shared approval
callback panics inside a sub-agent's run. After the S2.8 fix (PR #9), the
child run aborts and rolls back correctly, but the parent receives the
failure as an ordinary error tool result and continues its loop — with the
same broken approval gate still governing the parent's own future HITL calls.

## Analysis

Decision: **propagate-when-shared.** A sub-agent's approval gate is shared
exactly when the sub-agent definition sets no callback of its own and
inherits the parent's (S2.11). When that shared gate panics, the failure is
fatal to the parent run too — the S2.8 rationale ("continuing the loop after
a faulty gate would be worse than failing loudly") applies to every run the
gate governs. When the sub-agent has its own callback, its panic is fatal
only to the sub-agent's run; the parent's gate is unaffected, so the parent
receives an ordinary error tool result and continues (failure isolation,
S6.11).

Sharing is static per sub-agent definition, and the maximum nesting depth of
one (S2.11) means propagation is a single child-to-parent hop.

Alternatives considered: always-propagate (rejected — aborting the parent
for a sub-agent-local gate's panic is overreach; the parent's gate is
healthy) and documented status quo (rejected — the parent keeps running
behind a gate known to be broken, contradicting the S2.8 rationale).

Mechanics: the sub-agent tool preserves the typed `*ApprovalPanicError` from
the child run only when the gate is shared, and flattens it to a plain error
otherwise. The parent's dispatch treats a typed approval-panic error surfaced
by any tool as fatal to the batch, feeding the existing abort-and-rollback
path from PR #9. Sibling tools already executing in the parent's batch
complete before the abort; their results are discarded with the rolled-back
turn.

## PEGS Impact

- **S2.8 (Human-in-the-Loop):** panic rule extended with sub-agent
  propagation semantics (shared gate fatal to parent; local gate isolated).
- **S2.11 (Sub-Agent Composition):** HITL propagation rule cross-references
  the shared-gate panic rule.
- **S6 (Verification):** added S6.39 (Sub-Agent Shared-Gate Approval Panic
  Propagation); S6.11 pass condition notes the shared-gate exception;
  coverage table updated for S2.8 and S2.11.
- Non-requirements artifacts: `agent/dispatch.go`, `agent/subagent.go`,
  tests, version bump to v0.2.0 (behavior change on the sub-agent failure
  path).
