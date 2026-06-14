package agent

import "context"

// subAgentRuntime carries request-scoped information that an Agent.Run makes
// available to the tools it dispatches, so a sub-agent tool can inherit the
// running agent's stream sink and approval gate and learn its own nesting
// depth without changing the ToolFunc signature (S2.11). It is propagated on
// the context.Context, the same channel the library already uses for
// cancellation and OTEL span propagation (S2.12).
type subAgentRuntime struct {
	// parentHandler is the EventHandler of the agent whose Run is executing.
	// A sub-agent tool may forward its child's events here, tagged with
	// attribution, to surface them on the parent's stream (S2.3). It may be
	// nil when the running agent was given no handler.
	parentHandler EventHandler

	// approval is the approval callback of the running agent's tool registry,
	// propagated to a sub-agent's HITL-flagged tools by default (S2.8).
	approval ApprovalCallback

	// depth is the nesting depth of the running agent: 0 for the top-level
	// agent, 1 for a sub-agent. A sub-agent tool launches its child at
	// depth+1 and refuses to run when depth is already at the limit (S2.11).
	depth int
}

// subAgentRuntimeKey is an unexported context key type so the runtime value
// cannot collide with keys from other packages.
type subAgentRuntimeKey struct{}

// withSubAgentRuntime returns a copy of ctx carrying rt.
func withSubAgentRuntime(ctx context.Context, rt subAgentRuntime) context.Context {
	return context.WithValue(ctx, subAgentRuntimeKey{}, rt)
}

// subAgentRuntimeFrom retrieves the runtime carried on ctx, if any.
func subAgentRuntimeFrom(ctx context.Context) (subAgentRuntime, bool) {
	rt, ok := ctx.Value(subAgentRuntimeKey{}).(subAgentRuntime)
	return rt, ok
}

// inheritedApprovalKey carries the parent's approval callback to a sub-agent's
// run on a context key distinct from subAgentRuntimeKey, so Agent.Run can
// overwrite the runtime (with the child registry's own approval and the child
// depth) without dropping the parent callback the propagation bridge needs.
type inheritedApprovalKey struct{}

// withInheritedApproval returns a copy of ctx carrying the parent approval
// callback for a sub-agent's HITL-flagged tools (S2.8).
func withInheritedApproval(ctx context.Context, cb ApprovalCallback) context.Context {
	return context.WithValue(ctx, inheritedApprovalKey{}, cb)
}

// inheritedApprovalFrom retrieves the propagated parent approval callback.
func inheritedApprovalFrom(ctx context.Context) (ApprovalCallback, bool) {
	cb, ok := ctx.Value(inheritedApprovalKey{}).(ApprovalCallback)
	return cb, ok
}

// SubAgentContext exposes the propagated sub-agent runtime to hand-authored
// sub-agent tools (the low-level escape hatch for S2.11/S3.5). Within a tool's
// Execute it returns the running agent's event handler (for optional stream
// forwarding via ForwardEvents), its approval callback (to propagate to a
// child's HITL tools), and the running agent's nesting depth (0 at top level).
// When called outside an Agent.Run — where no runtime has been propagated —
// it returns nil, nil, 0.
func SubAgentContext(ctx context.Context) (parent EventHandler, approval ApprovalCallback, depth int) {
	rt, ok := subAgentRuntimeFrom(ctx)
	if !ok {
		return nil, nil, 0
	}
	return rt.parentHandler, rt.approval, rt.depth
}
