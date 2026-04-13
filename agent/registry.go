package agent

import "github.com/anthropics/anthropic-sdk-go"

// ToolRegistry manages the set of tools available to an Agent.
// For M1, this is a stub with no tool registration — the full
// implementation will be added in M2 (Tool Use).
type ToolRegistry struct{}

// NewToolRegistry creates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{}
}

// Tools returns the tool definitions for the Anthropic API.
// Returns nil when no tools are registered.
func (r *ToolRegistry) Tools() []anthropic.ToolUnionParam {
	return nil
}
