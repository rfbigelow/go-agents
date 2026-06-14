package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"go.opentelemetry.io/otel/attribute"
)

// defaultMaxIterations caps the agent loop when Config.MaxIterations is
// zero (the Go zero value). A small finite cap keeps misconfigured demos
// from looping forever while remaining generous enough for typical tool
// workflows.
const defaultMaxIterations = 16

// Agent manages the agent loop, coordinating LLM communication via the
// Completer, tool dispatch via the ToolRegistry, and conversation history
// via ConversationState.
type Agent struct {
	completer    Completer
	registry     *ToolRegistry
	config       Config
	conversation ConversationState
	hooks        HookBundle
	log          *slog.Logger
}

// NewAgent creates an Agent with the given Completer, ToolRegistry, and Config.
func NewAgent(completer Completer, registry *ToolRegistry, config Config) *Agent {
	return &Agent{
		completer: completer,
		registry:  registry,
		config:    config,
		log:       config.logger(),
	}
}

// Conversation returns the current conversation state.
func (a *Agent) Conversation() []anthropic.MessageParam {
	return a.conversation.Messages()
}

// SetHooks replaces the Agent's hook bundle in full (S2.10). Hooks may
// be set or replaced at any time, including between Run calls; the
// agent loop reads the current bundle at each hook point. An empty
// bundle (HookBundle{}) disables all hooks.
func (a *Agent) SetHooks(b HookBundle) {
	a.hooks = b
}

// Hooks returns the Agent's current hook bundle (S2.10).
func (a *Agent) Hooks() HookBundle {
	return a.hooks
}

// EventHandler is a callback invoked for each streaming event during a Run.
type EventHandler func(Event)

// Run executes the agent loop for a single user message. The handler
// callback is invoked for each streaming event. Run blocks until the
// agent loop completes or an error occurs.
func (a *Agent) Run(ctx context.Context, message string, handler EventHandler) error {
	ctx, span := startSpan(ctx, "agent.run",
		attribute.String("agent.model", string(a.config.Model)),
	)

	// Propagate a sub-agent runtime so tools dispatched during this run can
	// inherit this agent's stream sink and approval gate and learn their
	// nesting depth (S2.11). The incoming depth, if any, was set by the
	// sub-agent tool that launched this agent as a child; the top-level run
	// sees no incoming runtime and starts at depth 0.
	depth := 0
	if prev, ok := subAgentRuntimeFrom(ctx); ok {
		depth = prev.depth
	}
	// Serialize the handler exposed to sub-agent tools so parallel sub-agents
	// forwarding to this shared stream never invoke it concurrently (S2.3).
	// The agent's own streaming during complete() does not overlap tool
	// dispatch, so it continues to call handler directly.
	var streamMu sync.Mutex
	serialized := handler
	if handler != nil {
		serialized = func(e Event) {
			streamMu.Lock()
			defer streamMu.Unlock()
			handler(e)
		}
	}
	ctx = withSubAgentRuntime(ctx, subAgentRuntime{
		parentHandler: serialized,
		approval:      a.registry.approval,
		depth:         depth,
	})

	var runErr error
	var turns int
	defer func() {
		span.SetAttributes(attribute.Int("agent.turn_count", turns))
		endSpan(span, runErr)
	}()

	a.log.InfoContext(ctx, "run started",
		logArgs(ctx, "model", string(a.config.Model))...,
	)

	// Capture the conversation length BEFORE appending the user message so
	// that a hook failure mid-Run can roll back to the last completed turn
	// (S2.10). This is distinct from the Completer-error rollback, which
	// only rolls back the user message on turn 0.
	startLen := a.conversation.Len()
	a.conversation.Append(anthropic.NewUserMessage(anthropic.NewTextBlock(message)))

	maxIter := a.config.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultMaxIterations
	}

	for turn := 0; turn < maxIter; turn++ {
		turns = turn + 1
		if err := ctx.Err(); err != nil {
			runErr = err
			return fmt.Errorf("agent run: %w", err)
		}

		req := a.buildRequest()

		// PreLLMCall gate (S2.10) — fires BEFORE the agent.llm_call span so
		// a Substitute decision doesn't create a misleading span for a call
		// that never went out.
		preDecision, preErr := a.invokePreLLMCall(ctx, PreLLMCallEvent{Request: req, Turn: turn})
		if preErr != nil {
			runErr = preErr
			return a.abortRun(ctx, startLen, turn, preErr)
		}

		var response anthropic.Message
		var err error
		switch d := preDecision.(type) {
		case PreLLMCallContinue:
			response, err = a.complete(ctx, req, handler, turn)
		case PreLLMCallModify:
			a.logHookAction(ctx, hookPointPreLLMCall, "modify", "turn", turn)
			response, err = a.complete(ctx, d.Request, handler, turn)
		case PreLLMCallSubstitute:
			a.logHookAction(ctx, hookPointPreLLMCall, "substitute", "turn", turn)
			response = d.Message
		case PreLLMCallAbort:
			abortErr := &HookAbortError{Hook: hookPointPreLLMCall, Reason: d.Reason}
			runErr = abortErr
			return a.abortRun(ctx, startLen, turn, abortErr)
		default:
			// Defensive — unknown decision type from a future or buggy hook.
			// Treat as Continue and log.
			a.log.WarnContext(ctx, "unknown PreLLMCall decision; treating as Continue",
				logArgs(ctx, "decision_type", fmt.Sprintf("%T", preDecision), "turn", turn)...,
			)
			response, err = a.complete(ctx, req, handler, turn)
		}
		if err != nil {
			if turn == 0 {
				a.conversation.Rollback(1)
			}
			runErr = err
			a.log.ErrorContext(ctx, "run failed",
				logArgs(ctx, "turn", turn, "error", err.Error())...,
			)
			return fmt.Errorf("agent run: %w", err)
		}

		a.conversation.Append(response.ToParam())

		if response.StopReason != anthropic.StopReasonToolUse {
			a.log.InfoContext(ctx, "run completed",
				logArgs(ctx,
					"stop_reason", string(response.StopReason),
					"turn_count", turns,
					"input_tokens", response.Usage.InputTokens,
					"output_tokens", response.Usage.OutputTokens,
					"cache_creation_input_tokens", response.Usage.CacheCreationInputTokens,
					"cache_read_input_tokens", response.Usage.CacheReadInputTokens,
				)...,
			)
			return nil
		}

		calls := extractToolCalls(response)
		if len(calls) == 0 {
			// Defensive: stop_reason=tool_use without tool_use blocks.
			// Treat as final.
			return nil
		}

		results, _, batchErr := a.executeToolBatch(ctx, calls, turn)
		if batchErr != nil {
			runErr = batchErr
			return a.abortRun(ctx, startLen, turn, batchErr)
		}
		blocks := make([]anthropic.ContentBlockParamUnion, len(results))
		for i, r := range results {
			blocks[i] = anthropic.NewToolResultBlock(r.ID, r.Content, r.IsError)
		}
		a.conversation.Append(anthropic.NewUserMessage(blocks...))
	}

	runErr = ErrMaxIterations
	a.log.ErrorContext(ctx, "run failed",
		logArgs(ctx, "turn_count", turns, "error", runErr.Error())...,
	)
	return fmt.Errorf("agent run: %w", ErrMaxIterations)
}

// extractToolCalls scans the accumulated assistant message for tool_use
// content blocks and returns their decoded form. Reads the union fields
// directly so test-constructed messages (which lack JSON.raw) work too.
func extractToolCalls(msg anthropic.Message) []ToolCall {
	var calls []ToolCall
	for _, block := range msg.Content {
		if block.Type != "tool_use" {
			continue
		}
		calls = append(calls, ToolCall{
			ID:    block.ID,
			Name:  block.Name,
			Input: block.Input,
		})
	}
	return calls
}

// buildRequest constructs a CompletionRequest from the Agent's config
// and current conversation state.
func (a *Agent) buildRequest() CompletionRequest {
	req := CompletionRequest{
		Messages:  a.conversation.Messages(),
		Model:     a.config.Model,
		MaxTokens: a.config.MaxTokens,
	}

	if a.config.System != "" {
		req.System = []anthropic.TextBlockParam{
			{Text: a.config.System},
		}
	}

	if tools := a.registry.Tools(); len(tools) > 0 {
		req.Tools = tools
	}

	if a.config.Temperature != nil {
		req.Temperature = a.config.Temperature
	}

	if a.config.Thinking != nil {
		req.Thinking = a.config.Thinking
	}

	if a.config.Effort != nil {
		req.Effort = a.config.Effort
	}

	if !a.config.DisablePromptCaching {
		cc := anthropic.NewCacheControlEphemeralParam()
		if len(req.System) > 0 {
			req.System[len(req.System)-1].CacheControl = cc
		}
		if len(req.Tools) > 0 {
			*req.Tools[len(req.Tools)-1].GetCacheControl() = cc
		}
		if len(req.Messages) >= 2 {
			idx := len(req.Messages) - 2
			msg := req.Messages[idx]
			if n := len(msg.Content); n > 0 {
				newContent := make([]anthropic.ContentBlockParamUnion, n)
				copy(newContent, msg.Content)
				setCacheControlOnContentBlock(&newContent[n-1], cc)
				msg.Content = newContent
				req.Messages[idx] = msg
			}
		}
	}

	return req
}

// setCacheControlOnContentBlock clones the set variant's struct and sets
// CacheControl on the clone, so the original (shared with conversation
// state) is not mutated.
func setCacheControlOnContentBlock(b *anthropic.ContentBlockParamUnion, cc anthropic.CacheControlEphemeralParam) {
	switch {
	case b.OfText != nil:
		c := *b.OfText
		c.CacheControl = cc
		b.OfText = &c
	case b.OfToolUse != nil:
		c := *b.OfToolUse
		c.CacheControl = cc
		b.OfToolUse = &c
	case b.OfToolResult != nil:
		c := *b.OfToolResult
		c.CacheControl = cc
		b.OfToolResult = &c
	}
}

// complete calls the Completer and streams events to the handler.
// Returns the accumulated Message on success.
func (a *Agent) complete(ctx context.Context, req CompletionRequest, handler EventHandler, turn int) (anthropic.Message, error) {
	ctx, span := startSpan(ctx, "agent.llm_call",
		attribute.String("agent.model", string(req.Model)),
		attribute.Int("agent.message_count", len(req.Messages)),
		attribute.Int("agent.turn", turn),
	)
	var callErr error
	defer func() { endSpan(span, callErr) }()

	a.log.DebugContext(ctx, "llm call started",
		logArgs(ctx, "message_count", len(req.Messages), "turn", turn)...,
	)

	stream, err := a.completer.Complete(ctx, req)
	if err != nil {
		callErr = err
		return anthropic.Message{}, fmt.Errorf("completing: %w", err)
	}
	defer stream.Close()

	for stream.Next() {
		if handler != nil {
			handler(stream.Event())
		}
	}

	if err := stream.Err(); err != nil {
		callErr = err
		return anthropic.Message{}, fmt.Errorf("streaming: %w", err)
	}

	msg := stream.Message()

	if handler != nil {
		handler(Event{Type: EventDone})
	}

	span.SetAttributes(
		attribute.Int64("agent.cache_creation_input_tokens", msg.Usage.CacheCreationInputTokens),
		attribute.Int64("agent.cache_read_input_tokens", msg.Usage.CacheReadInputTokens),
	)

	a.log.DebugContext(ctx, "llm call completed",
		logArgs(ctx,
			"stop_reason", string(msg.StopReason),
			"output_tokens", msg.Usage.OutputTokens,
			"cache_creation_input_tokens", msg.Usage.CacheCreationInputTokens,
			"cache_read_input_tokens", msg.Usage.CacheReadInputTokens,
			"turn", turn,
		)...,
	)

	return msg, nil
}

// ErrMaxIterations is returned when the agent loop exceeds the
// configured maximum iteration count.
var ErrMaxIterations = errors.New("maximum iterations exceeded")

// logHookAction logs at Info level when a hook returned a non-Continue
// decision and the loop took a corresponding non-default path. Observers
// can subscribe to "hook action" entries to see when deterministic
// logic influenced the run.
func (a *Agent) logHookAction(ctx context.Context, hook, action string, attrs ...any) {
	args := append([]any{"hook", hook, "action", action}, attrs...)
	a.log.InfoContext(ctx, "hook action", logArgs(ctx, args...)...)
}

// invokePreLLMCall wraps a PreLLMCall hook invocation with the nil-hook
// short-circuit, error categorization (HookHandlerError), and panic
// recovery (HookPanicError). Returns either a non-nil decision and nil
// error, or nil decision and a wrapped HookError.
func (a *Agent) invokePreLLMCall(ctx context.Context, ev PreLLMCallEvent) (decision PreLLMCallDecision, hookErr error) {
	if a.hooks.PreLLMCall == nil {
		return PreLLMCallContinue{}, nil
	}
	defer func() {
		if p := recover(); p != nil {
			decision = nil
			hookErr = &HookPanicError{Hook: hookPointPreLLMCall, Recovered: p}
		}
	}()
	d, err := a.hooks.PreLLMCall.PreLLMCall(ctx, ev)
	if err != nil {
		return nil, &HookHandlerError{Hook: hookPointPreLLMCall, Err: err}
	}
	return d, nil
}

// abortRun is the common cleanup path for hook failures (S2.10): it
// rolls the conversation back to the state captured at the start of
// Run, logs the failure, and returns the wrapped error to be returned
// from Run. The caller is responsible for setting runErr before
// returning so the deferred span close records the failure.
func (a *Agent) abortRun(ctx context.Context, startLen, turn int, err error) error {
	n := a.conversation.Len() - startLen
	if n > 0 {
		a.conversation.Rollback(n)
	}
	a.log.ErrorContext(ctx, "run failed",
		logArgs(ctx, "turn", turn, "error", err.Error())...,
	)
	return fmt.Errorf("agent run: %w", err)
}

// invokePreToolUse wraps a PreToolUse hook invocation with the
// nil-hook short-circuit, error categorization, and panic recovery.
// Returns either a non-nil decision and nil error, or nil decision and
// a wrapped HookError.
func (a *Agent) invokePreToolUse(ctx context.Context, ev PreToolUseEvent) (decision PreToolUseDecision, hookErr error) {
	if a.hooks.PreToolUse == nil {
		return PreToolUseContinue{}, nil
	}
	defer func() {
		if p := recover(); p != nil {
			decision = nil
			hookErr = &HookPanicError{Hook: hookPointPreToolUse, Recovered: p}
		}
	}()
	d, err := a.hooks.PreToolUse.PreToolUse(ctx, ev)
	if err != nil {
		return nil, &HookHandlerError{Hook: hookPointPreToolUse, Err: err}
	}
	return d, nil
}

// invokePostToolUse wraps a PostToolUse hook invocation with the
// nil-hook short-circuit, error categorization, and panic recovery.
// Returns either a non-nil decision and nil error, or nil decision and
// a wrapped HookError.
func (a *Agent) invokePostToolUse(ctx context.Context, ev PostToolUseEvent) (decision PostToolUseDecision, hookErr error) {
	if a.hooks.PostToolUse == nil {
		return PostToolUseContinue{}, nil
	}
	defer func() {
		if p := recover(); p != nil {
			decision = nil
			hookErr = &HookPanicError{Hook: hookPointPostToolUse, Recovered: p}
		}
	}()
	d, err := a.hooks.PostToolUse.PostToolUse(ctx, ev)
	if err != nil {
		return nil, &HookHandlerError{Hook: hookPointPostToolUse, Err: err}
	}
	return d, nil
}

// executeToolBatch runs the per-call PreToolUse hook, dispatches the
// surviving (non-substituted) calls through the Tool Registry (which
// internally runs HITL and parallel execution), and returns results in
// the original call order along with a parallel synthesized-flag array
// indicating which results came from PreToolUse Substitute decisions
// rather than from actual tool execution.
//
// Substitute and Abort decisions at PreToolUse short-circuit before the
// registry is asked to dispatch — the registry-internal HITL gate
// (S2.8) is not invoked for substituted or aborted calls (S2.10
// ordering rule).
func (a *Agent) executeToolBatch(ctx context.Context, calls []ToolCall, turn int) ([]toolResult, []bool, error) {
	results := make([]toolResult, len(calls))
	synthesized := make([]bool, len(calls))

	// effectiveCalls tracks the call each tool "saw" — the original
	// ToolCall for Continue/Substitute, the rewritten call for Modify.
	// PostToolUse receives this so observers know what was actually
	// invoked.
	effectiveCalls := make([]ToolCall, len(calls))
	copy(effectiveCalls, calls)

	type survivor struct {
		idx  int
		call ToolCall
	}
	survivors := make([]survivor, 0, len(calls))

	for i, c := range calls {
		decision, hookErr := a.invokePreToolUse(ctx, PreToolUseEvent{Call: c, Turn: turn})
		if hookErr != nil {
			return nil, nil, hookErr
		}
		switch d := decision.(type) {
		case PreToolUseContinue:
			survivors = append(survivors, survivor{idx: i, call: c})
		case PreToolUseModify:
			a.logHookAction(ctx, hookPointPreToolUse, "modify", "turn", turn, "tool_name", c.Name, "tool_id", c.ID)
			modified := c
			modified.Input = d.Input
			survivors = append(survivors, survivor{idx: i, call: modified})
			effectiveCalls[i] = modified
		case PreToolUseSubstitute:
			a.logHookAction(ctx, hookPointPreToolUse, "substitute", "turn", turn, "tool_name", c.Name, "tool_id", c.ID)
			// Force the result's ID to the call's ID so the tool_result
			// block correlates correctly. The hook's supplied ID is
			// informational only (see ToolResult.ID).
			results[i] = toolResult{
				ID:      c.ID,
				Content: d.Result.Content,
				IsError: d.Result.IsError,
			}
			synthesized[i] = true
		case PreToolUseAbort:
			return nil, nil, &HookAbortError{Hook: hookPointPreToolUse, Reason: d.Reason}
		default:
			a.log.WarnContext(ctx, "unknown PreToolUse decision; treating as Continue",
				logArgs(ctx, "decision_type", fmt.Sprintf("%T", decision), "turn", turn, "tool_name", c.Name)...,
			)
			survivors = append(survivors, survivor{idx: i, call: c})
		}
	}

	if len(survivors) > 0 {
		survivorCalls := make([]ToolCall, len(survivors))
		for i, s := range survivors {
			survivorCalls[i] = s.call
		}
		dispatchResults := a.registry.dispatch(ctx, survivorCalls, a.log)
		for i, r := range dispatchResults {
			results[survivors[i].idx] = r
		}
	}

	// PostToolUse: per-call, serial. Fires for every call (substituted
	// or executed). The Synthesized flag rides on the event payload; a
	// Modify decision does not clear it (sticky for observers).
	for i, c := range effectiveCalls {
		decision, hookErr := a.invokePostToolUse(ctx, PostToolUseEvent{
			Call: c,
			Result: ToolResult{
				ID:      results[i].ID,
				Content: results[i].Content,
				IsError: results[i].IsError,
			},
			Synthesized: synthesized[i],
			Turn:        turn,
		})
		if hookErr != nil {
			return nil, nil, hookErr
		}
		switch d := decision.(type) {
		case PostToolUseContinue:
			// no-op — leave results[i] in place
		case PostToolUseModify:
			a.logHookAction(ctx, hookPointPostToolUse, "modify",
				"turn", turn, "tool_name", c.Name, "tool_id", c.ID, "synthesized", synthesized[i])
			results[i] = toolResult{
				ID:      c.ID,
				Content: d.Result.Content,
				IsError: d.Result.IsError,
			}
			// synthesized[i] intentionally NOT cleared — the flag is
			// sticky for downstream observers (tracing, logging).
		case PostToolUseAbort:
			return nil, nil, &HookAbortError{Hook: hookPointPostToolUse, Reason: d.Reason}
		default:
			a.log.WarnContext(ctx, "unknown PostToolUse decision; treating as Continue",
				logArgs(ctx, "decision_type", fmt.Sprintf("%T", decision), "turn", turn, "tool_name", c.Name)...,
			)
		}
	}

	return results, synthesized, nil
}
