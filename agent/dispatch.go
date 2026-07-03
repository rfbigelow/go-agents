package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"go.opentelemetry.io/otel/attribute"
)

// ApprovalPanicError is returned from Run when the HITL approval
// callback panics (S2.8). Unlike a tool-execution panic — which is
// recovered per call and surfaced to the LLM as an error result — a
// broken approval gate is fatal to the run: no tools in the batch
// execute, and the partial turn is rolled back so conversation state is
// preserved up to the last completed turn. The recovered value is
// preserved verbatim; if it is an error value, Unwrap exposes it.
type ApprovalPanicError struct {
	ToolName  string
	ToolID    string
	Recovered any
}

// Error implements error.
func (e *ApprovalPanicError) Error() string {
	return fmt.Sprintf("approval callback panicked for tool %q: %v", e.ToolName, e.Recovered)
}

// Unwrap exposes the recovered value when it satisfies the error
// interface, allowing errors.Is/errors.As to walk into it. Returns nil
// for non-error panics (e.g., string panics).
func (e *ApprovalPanicError) Unwrap() error {
	if err, ok := e.Recovered.(error); ok {
		return err
	}
	return nil
}

// invokeApproval runs the approval callback for a single HITL-flagged
// call, converting a panic into an *ApprovalPanicError (S2.8). A
// callback error is returned as-is and means denial, not abort.
func (r *ToolRegistry) invokeApproval(ctx context.Context, call ToolCall) (approved bool, err error) {
	defer func() {
		if p := recover(); p != nil {
			approved = false
			err = &ApprovalPanicError{ToolName: call.Name, ToolID: call.ID, Recovered: p}
		}
	}()
	return r.approval(ctx, call)
}

// dispatch resolves tool calls, runs HITL approvals sequentially, and then
// executes approved + non-HITL tools in parallel. The only non-nil error
// it returns is an *ApprovalPanicError (S2.8), from either of two sources:
// this registry's own approval pass panicking — which aborts before any
// tool in the batch executes — or a sub-agent tool surfacing a panic of
// the shared parent gate, in which case siblings already executing
// complete first and their results are discarded. Every other failure
// mode (unknown tool, denial, execution error, recovered tool panic)
// surfaces as an IsError toolResult so the LLM can adapt. Results are
// returned in the input order.
//
// Per S2.5: errors are isolated per call, siblings continue; tools
// inherit the enclosing ctx (no per-tool timeout); panics are recovered
// and logged.
func (r *ToolRegistry) dispatch(ctx context.Context, calls []ToolCall, log *slog.Logger) ([]toolResult, error) {
	ctx, span := startSpan(ctx, "agent.tool_dispatch",
		attribute.Int("tool.count", len(calls)),
	)
	var dispatchErr error
	defer func() { endSpan(span, dispatchErr) }()

	log.InfoContext(ctx, "dispatch started",
		logArgs(ctx, "tool_count", len(calls))...,
	)

	type slot struct {
		call     ToolCall
		tool     Tool
		known    bool
		approved bool
	}

	slots := make([]slot, len(calls))
	for i, c := range calls {
		t, ok := r.tools[c.Name]
		slots[i] = slot{
			call:     c,
			tool:     t,
			known:    ok,
			approved: ok && !t.HITL,
		}
	}

	// Sequential approval pass for HITL-flagged tools so callbacks don't
	// interleave in the UI layer. A callback panic aborts the whole
	// dispatch before any tool executes (S2.8) — the run rolls back the
	// partial turn rather than continuing behind a broken approval gate.
	for i := range slots {
		if !slots[i].known || !slots[i].tool.HITL {
			continue
		}
		ok, err := r.invokeApproval(ctx, slots[i].call)
		var panicErr *ApprovalPanicError
		if errors.As(err, &panicErr) {
			log.ErrorContext(ctx, "approval callback panic",
				logArgs(ctx, "tool_name", slots[i].call.Name, "tool_id", slots[i].call.ID, "panic", fmt.Sprint(panicErr.Recovered))...,
			)
			dispatchErr = err
			return nil, dispatchErr
		}
		if err != nil || !ok {
			slots[i].approved = false
			log.InfoContext(ctx, "tool denied",
				logArgs(ctx, "tool_name", slots[i].call.Name, "tool_id", slots[i].call.ID)...,
			)
			continue
		}
		slots[i].approved = true
	}

	results := make([]toolResult, len(calls))
	var wg sync.WaitGroup
	var errCount int64
	var fatalErr error
	var errMu sync.Mutex
	incErr := func() {
		errMu.Lock()
		errCount++
		errMu.Unlock()
	}
	// recordFatal keeps the first shared-gate approval panic surfaced by a
	// tool (S2.8); it aborts the whole dispatch after the batch drains.
	recordFatal := func(err error) {
		errMu.Lock()
		if fatalErr == nil {
			fatalErr = err
		}
		errMu.Unlock()
	}

	for i := range slots {
		if !slots[i].known {
			results[i] = toolResult{
				ID:      slots[i].call.ID,
				Content: fmt.Sprintf("unknown tool %q", slots[i].call.Name),
				IsError: true,
			}
			log.ErrorContext(ctx, "unknown tool",
				logArgs(ctx, "tool_name", slots[i].call.Name, "tool_id", slots[i].call.ID)...,
			)
			incErr()
			continue
		}
		if !slots[i].approved {
			results[i] = toolResult{
				ID:      slots[i].call.ID,
				Content: "tool call denied by approval callback",
				IsError: true,
			}
			incErr()
			continue
		}

		i := i
		s := slots[i]
		wg.Go(func() {
			defer func() {
				if p := recover(); p != nil {
					results[i] = toolResult{
						ID:      s.call.ID,
						Content: fmt.Sprintf("tool panicked: %v", p),
						IsError: true,
					}
					log.ErrorContext(ctx, "tool panic recovered",
						logArgs(ctx, "tool_name", s.call.Name, "tool_id", s.call.ID, "panic", fmt.Sprint(p))...,
					)
					incErr()
				}
			}()
			out, err := r.executeTool(ctx, s.tool, s.call, log)
			if err != nil {
				var panicErr *ApprovalPanicError
				if errors.As(err, &panicErr) {
					// A sub-agent sharing this run's approval gate saw it
					// panic (S2.8): fatal here too, not an LLM-visible
					// result.
					recordFatal(err)
				}
				results[i] = toolResult{
					ID:      s.call.ID,
					Content: err.Error(),
					IsError: true,
				}
				incErr()
				return
			}
			results[i] = toolResult{ID: s.call.ID, Content: out}
		})
	}
	wg.Wait()

	if fatalErr != nil {
		var panicErr *ApprovalPanicError
		errors.As(fatalErr, &panicErr)
		log.ErrorContext(ctx, "approval callback panic",
			logArgs(ctx, "tool_name", panicErr.ToolName, "tool_id", panicErr.ToolID, "panic", fmt.Sprint(panicErr.Recovered))...,
		)
		dispatchErr = fatalErr
		return nil, dispatchErr
	}

	log.InfoContext(ctx, "dispatch completed",
		logArgs(ctx, "tool_count", len(calls), "error_count", errCount)...,
	)
	return results, nil
}

// executeTool runs a single tool under its own span, recording the outcome.
func (r *ToolRegistry) executeTool(ctx context.Context, t Tool, call ToolCall, log *slog.Logger) (string, error) {
	ctx, span := startSpan(ctx, "agent.tool."+t.Name,
		attribute.String("tool.name", t.Name),
		attribute.Bool("tool.hitl", t.HITL),
	)
	var toolErr error
	defer func() { endSpan(span, toolErr) }()

	log.DebugContext(ctx, "tool started",
		logArgs(ctx, "tool_name", t.Name, "tool_id", call.ID)...,
	)

	out, err := t.Execute(ctx, call.Input)
	if err != nil {
		toolErr = err
		log.ErrorContext(ctx, "tool error",
			logArgs(ctx, "tool_name", t.Name, "tool_id", call.ID, "error", err.Error())...,
		)
		return "", err
	}

	log.InfoContext(ctx, "tool completed",
		logArgs(ctx, "tool_name", t.Name, "tool_id", call.ID)...,
	)
	return out, nil
}
