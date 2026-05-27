package agent

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
)

// Loop hooks let an application interpose deterministic, non-LLM logic at
// well-defined points in the agent loop (S2.10). Three hook points are
// defined: PreLLMCall, PreToolUse, and PostToolUse. Each point has its own
// typed handler interface and its own sealed decision type. At most one
// handler may be registered per point.
//
// Hooks fire synchronously and block the loop until they return. The library
// imposes no timeout; cancellation is via the context.Context passed to Run.
// Hooks are expected to be fast (machine-speed deterministic logic, not user
// prompts — see ApprovalCallback for the human-latency contract).
//
// Hooks do not subsume the observation surfaces: streaming events
// (EventHandler), tracing (OTEL spans), and structured logging continue to
// serve observability. Hooks are the only mutation surface.
//
// Sub-agent inheritance is intentionally not specified: each Agent has its
// own HookBundle (empty by default), and a tool that spawns a sub-agent may
// call SetHooks on the child before invoking Run.

// HookBundle is the record of optional handlers attached to an Agent for
// the three hook points. Any subset of handlers may be present; an empty
// bundle (the default on a newly-constructed Agent) means no hooks fire.
// Replace the bundle in full via Agent.SetHooks — there is no
// partial-update command.
type HookBundle struct {
	PreLLMCall  PreLLMCallHook
	PreToolUse  PreToolUseHook
	PostToolUse PostToolUseHook
}

// ToolResult is the per-call outcome carried into and out of PostToolUse,
// and the payload of a PreToolUse Substitute decision. Mirrors the
// per-call shape that flows from the Tool Registry back into the agentic
// loop's tool_result block.
type ToolResult struct {
	// ID is the tool_use_id this result corresponds to. On a
	// PostToolUseEvent it is the originating ToolCall.ID. When supplied
	// in a decision return (PreToolUseSubstitute, PostToolUseModify) it
	// is informational only — the agent forces the resulting
	// tool_result block to carry the originating ToolCall.ID so the LLM
	// can correlate the result.
	ID string

	// Content is the textual result the LLM will see in the tool_result
	// block.
	Content string

	// IsError indicates that the tool result represents a failure (the
	// tool errored, panicked, was denied, or is unknown). The LLM uses
	// this signal to adapt.
	IsError bool
}

// PreLLMCallHook fires before each LLM call. Returning Substitute skips
// the Completer entirely; returning Abort terminates the run with the
// carried reason.
type PreLLMCallHook interface {
	PreLLMCall(ctx context.Context, event PreLLMCallEvent) (PreLLMCallDecision, error)
}

// PreLLMCallHookFunc adapts a plain function to PreLLMCallHook.
type PreLLMCallHookFunc func(ctx context.Context, event PreLLMCallEvent) (PreLLMCallDecision, error)

// PreLLMCall implements PreLLMCallHook.
func (f PreLLMCallHookFunc) PreLLMCall(ctx context.Context, event PreLLMCallEvent) (PreLLMCallDecision, error) {
	return f(ctx, event)
}

// PreLLMCallEvent is the payload delivered to a PreLLMCallHook.
type PreLLMCallEvent struct {
	// Request is the CompletionRequest the Agent built for this turn.
	// PreLLMCallModify can supply a rewritten request to use instead.
	Request CompletionRequest

	// Turn is the zero-based agent-loop iteration index.
	Turn int
}

// PreLLMCallDecision is the sealed return type of a PreLLMCallHook.
type PreLLMCallDecision interface {
	isPreLLMCallDecision()
}

// PreLLMCallContinue proceeds with the original request.
type PreLLMCallContinue struct{}

func (PreLLMCallContinue) isPreLLMCallDecision() {}

// PreLLMCallModify proceeds with a rewritten CompletionRequest.
type PreLLMCallModify struct {
	Request CompletionRequest
}

func (PreLLMCallModify) isPreLLMCallDecision() {}

// PreLLMCallSubstitute skips the Completer and uses the supplied
// synthetic assistant Message as if it were the LLM's response. The
// agent loop processes the message normally, including any tool_use
// blocks it contains.
type PreLLMCallSubstitute struct {
	Message anthropic.Message
}

func (PreLLMCallSubstitute) isPreLLMCallDecision() {}

// PreLLMCallAbort terminates the run. The loop returns Reason wrapped as
// a HookAbortError. Conversation state is preserved up to the last
// completed turn (the partial turn that invoked the hook is not
// retained).
type PreLLMCallAbort struct {
	Reason error
}

func (PreLLMCallAbort) isPreLLMCallDecision() {}

// PreToolUseHook fires before each tool call, ahead of the HITL approval
// gate. Returning Substitute or Abort short-circuits before the human is
// bothered (S2.10 ordering rule).
type PreToolUseHook interface {
	PreToolUse(ctx context.Context, event PreToolUseEvent) (PreToolUseDecision, error)
}

// PreToolUseHookFunc adapts a plain function to PreToolUseHook.
type PreToolUseHookFunc func(ctx context.Context, event PreToolUseEvent) (PreToolUseDecision, error)

// PreToolUse implements PreToolUseHook.
func (f PreToolUseHookFunc) PreToolUse(ctx context.Context, event PreToolUseEvent) (PreToolUseDecision, error) {
	return f(ctx, event)
}

// PreToolUseEvent is the payload delivered to a PreToolUseHook.
type PreToolUseEvent struct {
	// Call is the tool-use request as decoded from the LLM response.
	Call ToolCall

	// Turn is the zero-based agent-loop iteration index.
	Turn int
}

// PreToolUseDecision is the sealed return type of a PreToolUseHook.
type PreToolUseDecision interface {
	isPreToolUseDecision()
}

// PreToolUseContinue dispatches the tool with the original arguments.
type PreToolUseContinue struct{}

func (PreToolUseContinue) isPreToolUseDecision() {}

// PreToolUseModify dispatches the tool with rewritten arguments. The new
// Input replaces the original ToolCall.Input for both HITL approval (if
// applicable) and tool execution.
type PreToolUseModify struct {
	Input []byte
}

func (PreToolUseModify) isPreToolUseDecision() {}

// PreToolUseSubstitute skips tool dispatch entirely and uses the supplied
// synthetic ToolResult. The HITL approval callback is not invoked. The
// synthetic result still flows through PostToolUse, carrying
// Synthesized: true on the event payload.
type PreToolUseSubstitute struct {
	Result ToolResult
}

func (PreToolUseSubstitute) isPreToolUseDecision() {}

// PreToolUseAbort terminates the run with Reason wrapped as a
// HookAbortError. The HITL approval callback is not invoked. State is
// preserved up to the last completed turn.
type PreToolUseAbort struct {
	Reason error
}

func (PreToolUseAbort) isPreToolUseDecision() {}

// PostToolUseHook fires after each tool's result is known (whether
// executed or synthesized via PreToolUse). There is no Substitute
// decision — the tool has already executed by the time PostToolUse fires.
type PostToolUseHook interface {
	PostToolUse(ctx context.Context, event PostToolUseEvent) (PostToolUseDecision, error)
}

// PostToolUseHookFunc adapts a plain function to PostToolUseHook.
type PostToolUseHookFunc func(ctx context.Context, event PostToolUseEvent) (PostToolUseDecision, error)

// PostToolUse implements PostToolUseHook.
func (f PostToolUseHookFunc) PostToolUse(ctx context.Context, event PostToolUseEvent) (PostToolUseDecision, error) {
	return f(ctx, event)
}

// PostToolUseEvent is the payload delivered to a PostToolUseHook.
type PostToolUseEvent struct {
	// Call is the (possibly Modify-rewritten) tool-use request.
	Call ToolCall

	// Result is the tool's outcome — either produced by the tool's
	// Execute function or supplied synthetically via a PreToolUse
	// Substitute decision.
	Result ToolResult

	// Synthesized is true when this result came from a PreToolUse
	// Substitute decision (the tool did not execute). The flag is
	// observational: it is sticky across a PostToolUse Modify decision
	// for downstream observers (tracing, logging) but does not flow into
	// the Anthropic tool_result block.
	Synthesized bool

	// Turn is the zero-based agent-loop iteration index.
	Turn int
}

// PostToolUseDecision is the sealed return type of a PostToolUseHook.
type PostToolUseDecision interface {
	isPostToolUseDecision()
}

// PostToolUseContinue forwards the original result to the LLM.
type PostToolUseContinue struct{}

func (PostToolUseContinue) isPostToolUseDecision() {}

// PostToolUseModify forwards a rewritten result to the LLM. The
// Synthesized flag from the event is preserved on observability outputs
// even after a Modify (the flag is sticky).
type PostToolUseModify struct {
	Result ToolResult
}

func (PostToolUseModify) isPostToolUseDecision() {}

// PostToolUseAbort terminates the run with Reason wrapped as a
// HookAbortError. State is preserved up to the last completed turn.
type PostToolUseAbort struct {
	Reason error
}

func (PostToolUseAbort) isPostToolUseDecision() {}
