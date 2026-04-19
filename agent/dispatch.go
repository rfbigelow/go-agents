package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"go.opentelemetry.io/otel/attribute"
)

// dispatch resolves tool calls, runs HITL approvals sequentially, and then
// executes approved + non-HITL tools in parallel. It never returns a
// non-nil error — every failure mode (unknown tool, denial, execution
// error, recovered panic) surfaces as an IsError toolResult so the LLM can
// adapt. Results are returned in the input order.
//
// Per S2.5: errors are isolated per call, siblings continue; tools
// inherit the enclosing ctx (no per-tool timeout); panics are recovered
// and logged.
func (r *ToolRegistry) dispatch(ctx context.Context, calls []ToolCall, log *slog.Logger) []toolResult {
	ctx, span := startSpan(ctx, "agent.tool_dispatch",
		attribute.Int("tool.count", len(calls)),
	)
	defer endSpan(span, nil)

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
	// interleave in the UI layer.
	for i := range slots {
		if !slots[i].known || !slots[i].tool.HITL {
			continue
		}
		ok, err := r.approval(ctx, slots[i].call)
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
	var errMu sync.Mutex
	incErr := func() {
		errMu.Lock()
		errCount++
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
		wg.Add(1)
		go func() {
			defer wg.Done()
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
				results[i] = toolResult{
					ID:      s.call.ID,
					Content: err.Error(),
					IsError: true,
				}
				incErr()
				return
			}
			results[i] = toolResult{ID: s.call.ID, Content: out}
		}()
	}
	wg.Wait()

	log.InfoContext(ctx, "dispatch completed",
		logArgs(ctx, "tool_count", len(calls), "error_count", errCount)...,
	)
	return results
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
