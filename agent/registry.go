package agent

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// ToolRegistry manages the set of tools available to an Agent. The
// consuming application registers tools and (optionally) an approval
// callback during initialization and hands the registry to the Agent.
// The registry is effectively immutable after setup; dispatch does not
// mutate it.
type ToolRegistry struct {
	tools    map[string]Tool
	order    []string
	approval ApprovalCallback
}

// NewToolRegistry creates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. It enforces:
//   - Tool.Name must be non-empty.
//   - Tool.Execute must be non-nil.
//   - Tool names must be unique within the registry.
//   - HITL-flagged tools require an approval callback to be registered
//     (fail-fast — see S2.4).
func (r *ToolRegistry) Register(t Tool) error {
	if t.Name == "" {
		return ErrEmptyToolName
	}
	if t.Execute == nil {
		return ErrNilToolFunc
	}
	if _, exists := r.tools[t.Name]; exists {
		return ErrDuplicateTool
	}
	if t.HITL && r.approval == nil {
		return ErrNoApprovalCallback
	}
	r.tools[t.Name] = t
	r.order = append(r.order, t.Name)
	return nil
}

// SetApprovalCallback installs the callback used for HITL-flagged tools.
// Must be called before registering any HITL tool; subsequent HITL
// registrations will use this callback.
func (r *ToolRegistry) SetApprovalCallback(cb ApprovalCallback) {
	r.approval = cb
}

// Tools returns the Anthropic-facing tool definitions, in registration
// order. Returns nil when no tools are registered.
func (r *ToolRegistry) Tools() []anthropic.ToolUnionParam {
	if len(r.order) == 0 {
		return nil
	}
	out := make([]anthropic.ToolUnionParam, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		u := anthropic.ToolUnionParamOfTool(t.InputSchema, t.Name)
		if t.Description != "" {
			u.OfTool.Description = param.NewOpt(t.Description)
		}
		out = append(out, u)
	}
	return out
}
