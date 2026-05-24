package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// ---------- PreLLMCall (S6.27) ----------

func TestPreLLMCall_Continue(t *testing.T) {
	mock := &mockCompleter{response: "hello"}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	var hookSaw PreLLMCallEvent
	a.SetHooks(HookBundle{
		PreLLMCall: PreLLMCallHookFunc(func(_ context.Context, ev PreLLMCallEvent) (PreLLMCallDecision, error) {
			hookSaw = ev
			return PreLLMCallContinue{}, nil
		}),
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if hookSaw.Turn != 0 {
		t.Errorf("hook saw Turn=%d, want 0", hookSaw.Turn)
	}
	if len(mock.capturedRequests) != 1 {
		t.Fatalf("expected 1 completer call, got %d", len(mock.capturedRequests))
	}
	// Continue must pass the original request through unchanged.
	if mock.capturedRequests[0].MaxTokens != 100 {
		t.Errorf("captured MaxTokens = %d, want 100 (original)", mock.capturedRequests[0].MaxTokens)
	}
	if len(mock.capturedRequests[0].Messages) != 1 {
		t.Errorf("expected 1 message in completer request, got %d", len(mock.capturedRequests[0].Messages))
	}
}

func TestPreLLMCall_Modify(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	a.SetHooks(HookBundle{
		PreLLMCall: PreLLMCallHookFunc(func(_ context.Context, ev PreLLMCallEvent) (PreLLMCallDecision, error) {
			// Rewrite the request: prepend a synthetic user-context message.
			rewritten := ev.Request
			rewritten.Messages = append(
				[]anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("[ctx]"))},
				rewritten.Messages...,
			)
			rewritten.MaxTokens = 200
			return PreLLMCallModify{Request: rewritten}, nil
		}),
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(mock.capturedRequests) != 1 {
		t.Fatalf("expected 1 completer call, got %d", len(mock.capturedRequests))
	}
	if mock.capturedRequests[0].MaxTokens != 200 {
		t.Errorf("captured MaxTokens = %d, want 200 (modified)", mock.capturedRequests[0].MaxTokens)
	}
	if len(mock.capturedRequests[0].Messages) != 2 {
		t.Errorf("expected 2 messages (prepended ctx + user), got %d", len(mock.capturedRequests[0].Messages))
	}
}

func TestPreLLMCall_Substitute_TextOnly(t *testing.T) {
	mock := &mockCompleter{response: "should-not-be-called"}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	syntheticText := "canned response"
	a.SetHooks(HookBundle{
		PreLLMCall: PreLLMCallHookFunc(func(_ context.Context, _ PreLLMCallEvent) (PreLLMCallDecision, error) {
			return PreLLMCallSubstitute{Message: anthropic.Message{
				ID:         "msg_synthetic",
				Role:       "assistant",
				StopReason: anthropic.StopReasonEndTurn,
				Content: []anthropic.ContentBlockUnion{
					{Type: "text", Text: syntheticText},
				},
			}}, nil
		}),
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if mock.call != 0 {
		t.Fatalf("expected 0 completer calls (Substitute skips Completer), got %d", mock.call)
	}

	// Conversation: user, assistant(synthetic).
	conv := a.Conversation()
	if len(conv) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(conv))
	}
}

func TestPreLLMCall_Substitute_ToolUse(t *testing.T) {
	// Substitute a tool_use response on turn 0; the loop must dispatch the
	// tool and call the Completer for turn 1 to produce the final response.
	registry := NewToolRegistry()
	var toolCalled bool
	mustRegister(t, registry, Tool{
		Name: "ping",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			toolCalled = true
			return "pong", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Text: "final"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	calls := 0
	a.SetHooks(HookBundle{
		PreLLMCall: PreLLMCallHookFunc(func(_ context.Context, _ PreLLMCallEvent) (PreLLMCallDecision, error) {
			calls++
			if calls == 1 {
				// Turn 0: substitute a tool_use response.
				return PreLLMCallSubstitute{Message: anthropic.Message{
					ID:         "msg_synthetic",
					Role:       "assistant",
					StopReason: anthropic.StopReasonToolUse,
					Content: []anthropic.ContentBlockUnion{
						{Type: "tool_use", ID: "toolu_x", Name: "ping", Input: json.RawMessage(`{}`)},
					},
				}}, nil
			}
			// Turn 1+: continue normally.
			return PreLLMCallContinue{}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !toolCalled {
		t.Fatal("expected tool to be dispatched from synthesized tool_use response")
	}
	if mock.call != 1 {
		t.Fatalf("expected 1 completer call (turn 1 only — turn 0 substituted), got %d", mock.call)
	}
}

func TestPreLLMCall_Abort(t *testing.T) {
	mock := &mockCompleter{response: "should-not-be-called"}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	sentinel := errors.New("policy violation")
	a.SetHooks(HookBundle{
		PreLLMCall: PreLLMCallHookFunc(func(_ context.Context, _ PreLLMCallEvent) (PreLLMCallDecision, error) {
			return PreLLMCallAbort{Reason: sentinel}, nil
		}),
	})

	err := a.Run(context.Background(), "hi", nil)
	if err == nil {
		t.Fatal("expected Run to return an error")
	}

	var abortErr *HookAbortError
	if !errors.As(err, &abortErr) {
		t.Fatalf("expected errors.As to *HookAbortError, got %T: %v", err, err)
	}
	if abortErr.HookPoint() != hookPointPreLLMCall {
		t.Errorf("HookPoint = %q, want %q", abortErr.HookPoint(), hookPointPreLLMCall)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is sentinel failed; Unwrap should expose the reason")
	}

	// Conversation should be empty (turn-0 partial rollback).
	if got := len(a.Conversation()); got != 0 {
		t.Errorf("expected empty conversation after Abort on turn 0, got %d messages", got)
	}
	if mock.call != 0 {
		t.Errorf("expected 0 completer calls after Abort, got %d", mock.call)
	}
}

func TestPreLLMCall_Abort_MidLoopRollsBackEntireRun(t *testing.T) {
	// Turn 0 succeeds with a tool_use; turn 1's PreLLMCall aborts. The
	// conversation must roll back to "before this Run started" — empty.
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "ok",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "fine", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t", Name: "ok", Input: json.RawMessage(`{}`)}}},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	turn := 0
	a.SetHooks(HookBundle{
		PreLLMCall: PreLLMCallHookFunc(func(_ context.Context, _ PreLLMCallEvent) (PreLLMCallDecision, error) {
			if turn == 0 {
				turn++
				return PreLLMCallContinue{}, nil
			}
			return PreLLMCallAbort{Reason: errors.New("stop now")}, nil
		}),
	})

	err := a.Run(context.Background(), "go", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var abortErr *HookAbortError
	if !errors.As(err, &abortErr) {
		t.Fatalf("expected *HookAbortError, got %T", err)
	}

	if got := len(a.Conversation()); got != 0 {
		t.Errorf("expected empty conversation after mid-loop Abort, got %d messages", got)
	}
}

// ---------- PreToolUse (S6.28) ----------

func TestPreToolUse_Continue(t *testing.T) {
	registry := NewToolRegistry()
	var receivedArgs json.RawMessage
	mustRegister(t, registry, Tool{
		Name: "echo",
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			receivedArgs = args
			return "ran", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "echo", Input: json.RawMessage(`{"k":"v"}`)}}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	var sawCall ToolCall
	a.SetHooks(HookBundle{
		PreToolUse: PreToolUseHookFunc(func(_ context.Context, ev PreToolUseEvent) (PreToolUseDecision, error) {
			sawCall = ev.Call
			return PreToolUseContinue{}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if sawCall.Name != "echo" {
		t.Errorf("hook saw call %+v, want Name=echo", sawCall)
	}
	if string(receivedArgs) != `{"k":"v"}` {
		t.Errorf("tool received args %q, want original %q", string(receivedArgs), `{"k":"v"}`)
	}
}

func TestPreToolUse_Modify(t *testing.T) {
	registry := NewToolRegistry()
	var receivedArgs json.RawMessage
	mustRegister(t, registry, Tool{
		Name: "echo",
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			receivedArgs = args
			return "ran", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "echo", Input: json.RawMessage(`{"k":"original"}`)}}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	rewritten := json.RawMessage(`{"k":"rewritten"}`)
	a.SetHooks(HookBundle{
		PreToolUse: PreToolUseHookFunc(func(_ context.Context, _ PreToolUseEvent) (PreToolUseDecision, error) {
			return PreToolUseModify{Input: rewritten}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if string(receivedArgs) != string(rewritten) {
		t.Errorf("tool received args %q, want rewritten %q", string(receivedArgs), string(rewritten))
	}
}

// TestPreToolUse_Substitute_BypassesHITL is the headline ordering test:
// when a PreToolUse hook returns Substitute, neither the tool's Execute
// nor the HITL approval callback are invoked. The synthetic result flows
// into the conversation.
func TestPreToolUse_Substitute_BypassesHITL(t *testing.T) {
	registry := NewToolRegistry()
	registry.SetApprovalCallback(func(_ context.Context, _ ToolCall) (bool, error) {
		t.Fatal("approval callback must not be invoked when PreToolUse returns Substitute")
		return false, nil
	})
	mustRegister(t, registry, Tool{
		Name: "sensitive",
		HITL: true,
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			t.Fatal("tool Execute must not be invoked when PreToolUse returns Substitute")
			return "", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "sensitive", Input: json.RawMessage(`{}`)}}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	a.SetHooks(HookBundle{
		PreToolUse: PreToolUseHookFunc(func(_ context.Context, _ PreToolUseEvent) (PreToolUseDecision, error) {
			return PreToolUseSubstitute{Result: ToolResult{
				Content: "synthetic",
			}}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// The conversation's user(tool_result) message should carry the
	// synthetic content with the original tool_use_id.
	conv := a.Conversation()
	if len(conv) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(conv))
	}
	toolResultMsg := conv[2]
	if len(toolResultMsg.Content) != 1 {
		t.Fatalf("expected 1 tool_result block, got %d", len(toolResultMsg.Content))
	}
	resBlock := toolResultMsg.Content[0].OfToolResult
	if resBlock == nil {
		t.Fatalf("expected tool_result block, got %+v", toolResultMsg.Content[0])
	}
	if resBlock.ToolUseID != "t1" {
		t.Errorf("ToolUseID = %q, want %q (agent must force ID match)", resBlock.ToolUseID, "t1")
	}
	// Synthetic content should appear in the result text block.
	if len(resBlock.Content) == 0 || resBlock.Content[0].OfText == nil || resBlock.Content[0].OfText.Text != "synthetic" {
		t.Errorf("expected synthetic content, got %+v", resBlock.Content)
	}
}

func TestPreToolUse_Abort_BypassesHITL(t *testing.T) {
	registry := NewToolRegistry()
	registry.SetApprovalCallback(func(_ context.Context, _ ToolCall) (bool, error) {
		t.Fatal("approval callback must not be invoked when PreToolUse returns Abort")
		return false, nil
	})
	mustRegister(t, registry, Tool{
		Name: "sensitive",
		HITL: true,
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			t.Fatal("tool Execute must not be invoked when PreToolUse returns Abort")
			return "", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "sensitive", Input: json.RawMessage(`{}`)}}},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	sentinel := errors.New("policy denied")
	a.SetHooks(HookBundle{
		PreToolUse: PreToolUseHookFunc(func(_ context.Context, _ PreToolUseEvent) (PreToolUseDecision, error) {
			return PreToolUseAbort{Reason: sentinel}, nil
		}),
	})

	err := a.Run(context.Background(), "go", nil)
	if err == nil {
		t.Fatal("expected Run to return an error")
	}

	var abortErr *HookAbortError
	if !errors.As(err, &abortErr) {
		t.Fatalf("expected *HookAbortError, got %T: %v", err, err)
	}
	if abortErr.HookPoint() != hookPointPreToolUse {
		t.Errorf("HookPoint = %q, want %q", abortErr.HookPoint(), hookPointPreToolUse)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is sentinel failed")
	}

	// Mid-loop abort rolls back to start-of-Run (empty conversation).
	if got := len(a.Conversation()); got != 0 {
		t.Errorf("expected empty conversation after mid-loop Abort, got %d", got)
	}
}

func TestPreToolUse_HITLOrdering_ContinueRunsApproval(t *testing.T) {
	registry := NewToolRegistry()
	var approvalRan bool
	registry.SetApprovalCallback(func(_ context.Context, _ ToolCall) (bool, error) {
		approvalRan = true
		return true, nil
	})
	var executed bool
	mustRegister(t, registry, Tool{
		Name: "sensitive",
		HITL: true,
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			executed = true
			return "ran", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "sensitive", Input: json.RawMessage(`{}`)}}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	var hookRan bool
	a.SetHooks(HookBundle{
		PreToolUse: PreToolUseHookFunc(func(_ context.Context, _ PreToolUseEvent) (PreToolUseDecision, error) {
			hookRan = true
			return PreToolUseContinue{}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !hookRan {
		t.Fatal("expected hook to run")
	}
	if !approvalRan {
		t.Fatal("expected approval callback to run after Continue decision")
	}
	if !executed {
		t.Fatal("expected tool to execute after approval")
	}
}

// ---------- PostToolUse (S6.29) ----------

func TestPostToolUse_Continue(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "echo",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "raw", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "echo", Input: json.RawMessage(`{}`)}}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	var seenEvent PostToolUseEvent
	a.SetHooks(HookBundle{
		PostToolUse: PostToolUseHookFunc(func(_ context.Context, ev PostToolUseEvent) (PostToolUseDecision, error) {
			seenEvent = ev
			return PostToolUseContinue{}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if seenEvent.Result.Content != "raw" {
		t.Errorf("hook saw Result.Content = %q, want %q", seenEvent.Result.Content, "raw")
	}
	if seenEvent.Synthesized {
		t.Errorf("hook saw Synthesized=true, want false (no PreToolUse Substitute)")
	}

	// Conversation tool_result block must carry "raw".
	conv := a.Conversation()
	resBlock := conv[2].Content[0].OfToolResult
	if resBlock == nil || len(resBlock.Content) == 0 || resBlock.Content[0].OfText == nil || resBlock.Content[0].OfText.Text != "raw" {
		t.Errorf("expected tool_result content %q, got %+v", "raw", resBlock.Content)
	}
}

func TestPostToolUse_Modify(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "echo",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "raw-secret", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "echo", Input: json.RawMessage(`{}`)}}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	a.SetHooks(HookBundle{
		PostToolUse: PostToolUseHookFunc(func(_ context.Context, ev PostToolUseEvent) (PostToolUseDecision, error) {
			return PostToolUseModify{Result: ToolResult{Content: "redacted"}}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	conv := a.Conversation()
	resBlock := conv[2].Content[0].OfToolResult
	if resBlock == nil || len(resBlock.Content) == 0 || resBlock.Content[0].OfText == nil || resBlock.Content[0].OfText.Text != "redacted" {
		t.Errorf("expected tool_result content %q (modified), got %+v", "redacted", resBlock.Content)
	}
	if resBlock.ToolUseID != "t1" {
		t.Errorf("ToolUseID = %q, want %q (agent must force ID match)", resBlock.ToolUseID, "t1")
	}
}

func TestPostToolUse_Abort(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "echo",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "raw", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "echo", Input: json.RawMessage(`{}`)}}},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	sentinel := errors.New("post-policy violation")
	a.SetHooks(HookBundle{
		PostToolUse: PostToolUseHookFunc(func(_ context.Context, _ PostToolUseEvent) (PostToolUseDecision, error) {
			return PostToolUseAbort{Reason: sentinel}, nil
		}),
	})

	err := a.Run(context.Background(), "go", nil)
	if err == nil {
		t.Fatal("expected Run to return an error")
	}

	var abortErr *HookAbortError
	if !errors.As(err, &abortErr) {
		t.Fatalf("expected *HookAbortError, got %T: %v", err, err)
	}
	if abortErr.HookPoint() != hookPointPostToolUse {
		t.Errorf("HookPoint = %q, want %q", abortErr.HookPoint(), hookPointPostToolUse)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is sentinel failed")
	}

	// Mid-loop abort rolls back to start-of-Run (empty conversation).
	if got := len(a.Conversation()); got != 0 {
		t.Errorf("expected empty conversation after mid-loop Abort, got %d", got)
	}
}

// TestPostToolUse_SynthesizedFlagSet verifies that when PreToolUse
// returned Substitute, the subsequent PostToolUse event payload carries
// Synthesized: true (S2.10 + S6.29).
func TestPostToolUse_SynthesizedFlagSet(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "tool",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			t.Fatal("Execute must not run when PreToolUse substitutes")
			return "", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "tool", Input: json.RawMessage(`{}`)}}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	var post PostToolUseEvent
	a.SetHooks(HookBundle{
		PreToolUse: PreToolUseHookFunc(func(_ context.Context, _ PreToolUseEvent) (PreToolUseDecision, error) {
			return PreToolUseSubstitute{Result: ToolResult{Content: "synthetic"}}, nil
		}),
		PostToolUse: PostToolUseHookFunc(func(_ context.Context, ev PostToolUseEvent) (PostToolUseDecision, error) {
			post = ev
			return PostToolUseContinue{}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !post.Synthesized {
		t.Error("PostToolUse event must carry Synthesized=true after PreToolUse Substitute")
	}
	if post.Result.Content != "synthetic" {
		t.Errorf("post.Result.Content = %q, want %q", post.Result.Content, "synthetic")
	}
}

// TestPostToolUse_SynthesizedStickyAcrossModify verifies that a Modify
// decision at PostToolUse does not clear the agent's internal
// synthesized state. Since the flag is observational and we can only
// see it through the PostToolUse event payload, this test exercises the
// flag in a sub-batch where one call is substituted and the other is
// not — confirming both that the flag is set for substituted calls and
// not set for executed calls, even when Modify is applied to both.
func TestPostToolUse_SynthesizedStickyAcrossModify(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "executed",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "real", nil
		},
	})
	mustRegister(t, registry, Tool{
		Name: "subbed",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			t.Fatal("subbed.Execute must not run")
			return "", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{
				{ID: "t-exec", Name: "executed", Input: json.RawMessage(`{}`)},
				{ID: "t-sub", Name: "subbed", Input: json.RawMessage(`{}`)},
			}},
			{Text: "done"},
		},
	}
	logHandler := newCapturingHandler()
	a := NewAgent(mock, registry, Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Logger:    slog.New(logHandler),
	})

	type observed struct {
		toolName    string
		synthesized bool
	}
	var post []observed
	a.SetHooks(HookBundle{
		PreToolUse: PreToolUseHookFunc(func(_ context.Context, ev PreToolUseEvent) (PreToolUseDecision, error) {
			if ev.Call.Name == "subbed" {
				return PreToolUseSubstitute{Result: ToolResult{Content: "synth-content"}}, nil
			}
			return PreToolUseContinue{}, nil
		}),
		PostToolUse: PostToolUseHookFunc(func(_ context.Context, ev PostToolUseEvent) (PostToolUseDecision, error) {
			post = append(post, observed{toolName: ev.Call.Name, synthesized: ev.Synthesized})
			// Modify both — the flag must not be derived from whether we Modify.
			return PostToolUseModify{Result: ToolResult{Content: "modified-" + ev.Result.Content}}, nil
		}),
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(post) != 2 {
		t.Fatalf("expected 2 post-hook invocations, got %d", len(post))
	}
	// Order matches call order: executed first, subbed second.
	if post[0].toolName != "executed" || post[0].synthesized {
		t.Errorf("post[0] = %+v, want {executed false}", post[0])
	}
	if post[1].toolName != "subbed" || !post[1].synthesized {
		t.Errorf("post[1] = %+v, want {subbed true}", post[1])
	}
	// Modify applied to both — conversation reflects "modified-real" and "modified-synth-content".
	conv := a.Conversation()
	if len(conv) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(conv))
	}
	resBlocks := conv[2].Content
	if len(resBlocks) != 2 {
		t.Fatalf("expected 2 tool_result blocks, got %d", len(resBlocks))
	}
	got0 := resBlocks[0].OfToolResult.Content[0].OfText.Text
	got1 := resBlocks[1].OfToolResult.Content[0].OfText.Text
	if got0 != "modified-real" || got1 != "modified-synth-content" {
		t.Errorf("conversation results = [%q, %q], want [\"modified-real\", \"modified-synth-content\"]", got0, got1)
	}

	// Stickiness for observers: the post-hook Modify log records must
	// preserve the synthesized flag exactly as it entered the hook.
	// Find the two "hook action" / "post_tool_use" / "modify" log
	// records and check their synthesized attrs.
	logHandler.mu.Lock()
	defer logHandler.mu.Unlock()
	type logSeen struct {
		toolName    string
		synthesized bool
	}
	var logged []logSeen
	for _, r := range logHandler.records {
		if r.Message != "hook action" {
			continue
		}
		hookV, _ := getAttr(r, "hook")
		actV, _ := getAttr(r, "action")
		if hookV.String() != hookPointPostToolUse || actV.String() != "modify" {
			continue
		}
		toolV, _ := getAttr(r, "tool_name")
		synthV, _ := getAttr(r, "synthesized")
		logged = append(logged, logSeen{toolName: toolV.String(), synthesized: synthV.Bool()})
	}
	if len(logged) != 2 {
		t.Fatalf("expected 2 post-modify log records, got %d", len(logged))
	}
	if logged[0].toolName != "executed" || logged[0].synthesized {
		t.Errorf("log[0] = %+v, want {executed false}", logged[0])
	}
	if logged[1].toolName != "subbed" || !logged[1].synthesized {
		t.Errorf("log[1] = %+v, want {subbed true} — flag should remain sticky for observers", logged[1])
	}
}

// ---------- Cross-cutting handler-error and panic tests (S6.30, S6.31) ----------

// hookKind identifies one of the three hook points for table-driven tests.
type hookKind int

const (
	kindPreLLMCall hookKind = iota
	kindPreToolUse
	kindPostToolUse
)

func (k hookKind) String() string {
	switch k {
	case kindPreLLMCall:
		return "pre_llm_call"
	case kindPreToolUse:
		return "pre_tool_use"
	case kindPostToolUse:
		return "post_tool_use"
	}
	return "?"
}

// agentForHookKind builds an Agent with a tool registered and a mocked
// completer scripted to produce one tool-use turn. Suitable for
// exercising any of the three hook points: PreLLMCall fires on turn 0
// before any tool call; PreToolUse and PostToolUse fire around the tool
// dispatch in turn 0's loop iteration.
func agentForHookKind(t *testing.T, logger *slog.Logger) *Agent {
	t.Helper()
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "echo",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "ok", nil
		},
	})
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "echo", Input: json.RawMessage(`{}`)}}},
			{Text: "done"},
		},
	}
	cfg := Config{Model: "claude-sonnet-4-5", MaxTokens: 100}
	if logger != nil {
		cfg.Logger = logger
	}
	return NewAgent(mock, registry, cfg)
}

// installFailingHook wires the appropriate hook on the bundle so that
// invocation produces handlerErr (if non-nil) or panics with panicVal
// (if non-nil). Exactly one of handlerErr and panicVal must be set.
func installFailingHook(bundle *HookBundle, kind hookKind, handlerErr error, panicVal any) {
	switch kind {
	case kindPreLLMCall:
		bundle.PreLLMCall = PreLLMCallHookFunc(func(_ context.Context, _ PreLLMCallEvent) (PreLLMCallDecision, error) {
			if panicVal != nil {
				panic(panicVal)
			}
			return PreLLMCallContinue{}, handlerErr
		})
	case kindPreToolUse:
		bundle.PreToolUse = PreToolUseHookFunc(func(_ context.Context, _ PreToolUseEvent) (PreToolUseDecision, error) {
			if panicVal != nil {
				panic(panicVal)
			}
			return PreToolUseContinue{}, handlerErr
		})
	case kindPostToolUse:
		bundle.PostToolUse = PostToolUseHookFunc(func(_ context.Context, _ PostToolUseEvent) (PostToolUseDecision, error) {
			if panicVal != nil {
				panic(panicVal)
			}
			return PostToolUseContinue{}, handlerErr
		})
	}
}

// TestHook_HandlerError_AllThreePoints (S6.31) — a hook handler returning
// a non-nil error is wrapped as *HookHandlerError, distinguishable via
// errors.As from *HookAbortError and *HookPanicError. State rolls back
// to the last completed turn.
func TestHook_HandlerError_AllThreePoints(t *testing.T) {
	for _, kind := range []hookKind{kindPreLLMCall, kindPreToolUse, kindPostToolUse} {
		t.Run(kind.String(), func(t *testing.T) {
			handlerErr := errors.New("dep unreachable")
			a := agentForHookKind(t, discardLogger())
			var bundle HookBundle
			installFailingHook(&bundle, kind, handlerErr, nil)
			a.SetHooks(bundle)

			err := a.Run(context.Background(), "go", nil)
			if err == nil {
				t.Fatal("expected error")
			}

			var hErr *HookHandlerError
			if !errors.As(err, &hErr) {
				t.Fatalf("expected errors.As to *HookHandlerError, got %T: %v", err, err)
			}
			if hErr.HookPoint() != kind.String() {
				t.Errorf("HookPoint = %q, want %q", hErr.HookPoint(), kind.String())
			}

			// Distinguishable: must NOT match Abort or Panic wrappers.
			var aErr *HookAbortError
			if errors.As(err, &aErr) {
				t.Errorf("errors.As to *HookAbortError must NOT match a handler-error")
			}
			var pErr *HookPanicError
			if errors.As(err, &pErr) {
				t.Errorf("errors.As to *HookPanicError must NOT match a handler-error")
			}

			// Underlying handler error is reachable via Unwrap.
			if !errors.Is(err, handlerErr) {
				t.Errorf("errors.Is handlerErr should match via Unwrap")
			}

			// Common HookError interface satisfied.
			var generic HookError
			if !errors.As(err, &generic) {
				t.Errorf("errors.As to HookError interface should match")
			}

			// Conversation rolled back to start-of-Run (empty).
			if got := len(a.Conversation()); got != 0 {
				t.Errorf("expected empty conversation after handler error, got %d", got)
			}
		})
	}
}

// TestHook_Panic_AllThreePoints (S6.30) — a hook handler that panics
// produces *HookPanicError, distinguishable via errors.As. The panic
// value is reachable via Recovered(). Log output records the panic
// details.
func TestHook_Panic_AllThreePoints(t *testing.T) {
	for _, kind := range []hookKind{kindPreLLMCall, kindPreToolUse, kindPostToolUse} {
		t.Run(kind.String(), func(t *testing.T) {
			handler := newCapturingHandler()
			a := agentForHookKind(t, slog.New(handler))
			var bundle HookBundle
			installFailingHook(&bundle, kind, nil, "gone wild")
			a.SetHooks(bundle)

			err := a.Run(context.Background(), "go", nil)
			if err == nil {
				t.Fatal("expected error")
			}

			var pErr *HookPanicError
			if !errors.As(err, &pErr) {
				t.Fatalf("expected errors.As to *HookPanicError, got %T: %v", err, err)
			}
			if pErr.HookPoint() != kind.String() {
				t.Errorf("HookPoint = %q, want %q", pErr.HookPoint(), kind.String())
			}
			if pErr.Recovered != "gone wild" {
				t.Errorf("Recovered = %v, want %q", pErr.Recovered, "gone wild")
			}

			// Distinguishable: must NOT match Abort or HandlerError wrappers.
			var aErr *HookAbortError
			if errors.As(err, &aErr) {
				t.Errorf("errors.As to *HookAbortError must NOT match a panic")
			}
			var hErr *HookHandlerError
			if errors.As(err, &hErr) {
				t.Errorf("errors.As to *HookHandlerError must NOT match a panic")
			}

			// Common HookError interface satisfied.
			var generic HookError
			if !errors.As(err, &generic) {
				t.Errorf("errors.As to HookError interface should match")
			}

			// Conversation rolled back to start-of-Run (empty).
			if got := len(a.Conversation()); got != 0 {
				t.Errorf("expected empty conversation after panic, got %d", got)
			}

			// Log captured the panic: at least one Error record must
			// have an "error" attr containing "panicked".
			errorRecs := handler.errorRecords()
			if len(errorRecs) == 0 {
				t.Fatal("expected at least one Error-level log record")
			}
			found := false
			for _, r := range errorRecs {
				if v, ok := getAttr(r, "error"); ok && strings.Contains(v.String(), "panicked") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected an Error log record with an \"error\" attr containing \"panicked\"")
			}
		})
	}
}

// ---------- SetHooks behavior (S2.10 Rules) ----------

func TestSetHooks_ReplaceBetweenRuns(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	// Run 1: no hooks.
	if err := a.Run(context.Background(), "first", nil); err != nil {
		t.Fatalf("Run 1 failed: %v", err)
	}

	// Run 2: install a hook between runs.
	var fired bool
	a.SetHooks(HookBundle{
		PreLLMCall: PreLLMCallHookFunc(func(_ context.Context, _ PreLLMCallEvent) (PreLLMCallDecision, error) {
			fired = true
			return PreLLMCallContinue{}, nil
		}),
	})
	if err := a.Run(context.Background(), "second", nil); err != nil {
		t.Fatalf("Run 2 failed: %v", err)
	}
	if !fired {
		t.Error("expected hook installed between runs to fire on Run 2")
	}
}

func TestSetHooks_EmptyBundleDisablesHooks(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	var fireCount int
	a.SetHooks(HookBundle{
		PreLLMCall: PreLLMCallHookFunc(func(_ context.Context, _ PreLLMCallEvent) (PreLLMCallDecision, error) {
			fireCount++
			return PreLLMCallContinue{}, nil
		}),
	})

	if err := a.Run(context.Background(), "first", nil); err != nil {
		t.Fatalf("Run 1 failed: %v", err)
	}
	if fireCount != 1 {
		t.Fatalf("expected 1 fire on Run 1, got %d", fireCount)
	}

	// Clear the bundle — subsequent runs must not fire the hook.
	a.SetHooks(HookBundle{})
	if err := a.Run(context.Background(), "second", nil); err != nil {
		t.Fatalf("Run 2 failed: %v", err)
	}
	if fireCount != 1 {
		t.Errorf("expected fire count to remain 1 after clearing bundle, got %d", fireCount)
	}

	// Hooks() reflects the empty bundle.
	if h := a.Hooks(); h.PreLLMCall != nil || h.PreToolUse != nil || h.PostToolUse != nil {
		t.Errorf("Hooks() = %+v, want empty bundle", h)
	}
}

// ---------- helpers ----------

// capturingHandler is a slog.Handler that records every log record into
// an in-memory slice. Used by panic tests to assert the panic details
// were logged via the standard run-failure log entry.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func newCapturingHandler() *capturingHandler { return &capturingHandler{} }

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r)
	h.mu.Unlock()
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingHandler) errorRecords() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if r.Level == slog.LevelError {
			out = append(out, r)
		}
	}
	return out
}

// getAttr extracts a top-level attr by key from a slog.Record. Returns
// the value and true if found, otherwise zero value and false.
func getAttr(r slog.Record, key string) (slog.Value, bool) {
	var (
		val   slog.Value
		found bool
	)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			val = a.Value
			found = true
			return false
		}
		return true
	})
	return val, found
}

