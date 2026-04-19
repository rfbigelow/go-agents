package agent

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
)

// Tool is a registerable tool definition. The Anthropic SDK's native
// ToolInputSchemaParam is used directly so tool authors do not fight two
// schema representations.
type Tool struct {
	Name        string
	Description string
	InputSchema anthropic.ToolInputSchemaParam
	HITL        bool
	Execute     ToolFunc
}

// ToolFunc is the execution signature for a tool. The ctx carries
// cancellation and OTEL span propagation from Agent.Run. The args are the
// raw JSON input from the LLM; the tool decodes them into whatever type
// and validator it prefers. A non-nil error is reported to the LLM as an
// error tool_result.
type ToolFunc func(ctx context.Context, args json.RawMessage) (string, error)

// ToolCall is the decoded tool-use request handed to the approval callback
// and used internally by dispatch. Library-defined so callbacks never touch
// the SDK's ToolUseBlock type directly.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ApprovalCallback decides whether a HITL-flagged tool call may execute.
// It is invoked before any tool in the batch runs. Returning false, or a
// non-nil error, denies the call — the tool is not executed and an error
// tool_result is sent to the LLM so it can adapt.
type ApprovalCallback func(ctx context.Context, call ToolCall) (bool, error)

// toolResult is the per-call outcome produced by dispatch. It is not
// exported — only Agent.Run consumes it, converting each entry into an
// anthropic.NewToolResultBlock for the next conversation turn.
type toolResult struct {
	ID      string
	Content string
	IsError bool
}

// Registration errors.
var (
	ErrDuplicateTool      = errors.New("tool name already registered")
	ErrNoApprovalCallback = errors.New("HITL tool registered without approval callback")
	ErrEmptyToolName      = errors.New("tool name is empty")
	ErrNilToolFunc        = errors.New("tool Execute is nil")
)
