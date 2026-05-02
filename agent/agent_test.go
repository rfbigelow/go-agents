package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// scriptedResponse describes one Completer.Complete result. For the
// default case (plain text end_turn), set Text. For a tool_use turn,
// leave Text empty and populate ToolCalls. Set Thinking and/or Signature
// to prepend an Extended Thinking block to the assistant message and
// surface thinking_delta / signature_delta events through the stream.
type scriptedResponse struct {
	Text      string
	ToolCalls []scriptedToolCall
	Thinking  string
	Signature string
}

type scriptedToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// mockCompleter implements Completer for testing. In its simplest form
// (Response set) it returns a single end_turn response on every call — the
// shape M1 tests depend on. Set Responses for a multi-turn script; each
// Complete call consumes one entry in order. capturedRequests records
// every CompletionRequest passed to Complete in call order, supporting
// S6.25 / S6.26 passthrough assertions.
type mockCompleter struct {
	response         string
	err              error
	responses        []scriptedResponse
	call             int
	capturedRequests []CompletionRequest
}

func (m *mockCompleter) Complete(_ context.Context, req CompletionRequest) (*EventStream, error) {
	m.capturedRequests = append(m.capturedRequests, req)

	if m.err != nil {
		return nil, m.err
	}

	var r scriptedResponse
	if len(m.responses) > 0 {
		if m.call >= len(m.responses) {
			// Script exhausted — emit a generic end_turn so tests that
			// hit this accidentally fail loudly on assertion, not on
			// index-out-of-range.
			r = scriptedResponse{Text: "(script exhausted)"}
		} else {
			r = m.responses[m.call]
		}
		m.call++
	} else {
		r = scriptedResponse{Text: m.response}
	}

	return m.buildStream(r), nil
}

func (m *mockCompleter) buildStream(r scriptedResponse) *EventStream {
	msg := anthropic.Message{
		ID:    "msg_test",
		Role:  "assistant",
		Model: "claude-sonnet-4-5",
		Usage: anthropic.Usage{InputTokens: 10, OutputTokens: 5},
	}

	events := []testEvent{}
	if r.Thinking != "" || r.Signature != "" {
		msg.Content = append(msg.Content, anthropic.ContentBlockUnion{
			Type:      "thinking",
			Thinking:  r.Thinking,
			Signature: r.Signature,
		})
		if r.Thinking != "" {
			events = append(events, testEvent{Kind: "thinking", Value: r.Thinking})
		}
		if r.Signature != "" {
			events = append(events, testEvent{Kind: "signature", Value: r.Signature})
		}
	}
	if r.Text != "" {
		msg.Content = append(msg.Content, anthropic.ContentBlockUnion{
			Type: "text",
			Text: r.Text,
		})
		events = append(events, testEvent{Kind: "text", Value: r.Text})
	}
	for _, tc := range r.ToolCalls {
		msg.Content = append(msg.Content, anthropic.ContentBlockUnion{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Name,
			Input: tc.Input,
		})
	}
	if len(r.ToolCalls) > 0 {
		msg.StopReason = anthropic.StopReasonToolUse
	} else {
		msg.StopReason = anthropic.StopReasonEndTurn
	}

	// Round-trip through JSON so ContentBlockUnion.JSON.raw is populated.
	// Without this, Message.ToParam() projects each block via its AsXxx()
	// helper, which reads from JSON.raw — fields set directly on the
	// union (Thinking, Signature, ID, Name, Input) would otherwise be
	// silently dropped when the agent appends response.ToParam() to the
	// conversation.
	data, err := json.Marshal(msg)
	if err != nil {
		panic(fmt.Sprintf("buildStream marshal: %v", err))
	}
	var roundTripped anthropic.Message
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		panic(fmt.Sprintf("buildStream unmarshal: %v", err))
	}

	return NewTestEventStreamFromEvents(events, roundTripped)
}

func TestAgent_SimpleConversation(t *testing.T) {
	mock := &mockCompleter{response: "Hello! How can I help?"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		System:    "You are helpful.",
	})

	var received []string
	err := a.Run(context.Background(), "Hi there", func(e Event) {
		if e.Type == EventTextDelta {
			received = append(received, e.Text)
		}
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(received) != 1 || received[0] != "Hello! How can I help?" {
		t.Fatalf("expected [\"Hello! How can I help?\"], got %v", received)
	}

	conv := a.Conversation()
	if len(conv) != 2 {
		t.Fatalf("expected 2 messages in conversation, got %d", len(conv))
	}
}

func TestAgent_MultiTurnConversation(t *testing.T) {
	mock := &mockCompleter{response: "response"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
	})

	if err := a.Run(context.Background(), "first", nil); err != nil {
		t.Fatalf("Run 1 failed: %v", err)
	}
	if err := a.Run(context.Background(), "second", nil); err != nil {
		t.Fatalf("Run 2 failed: %v", err)
	}

	conv := a.Conversation()
	if len(conv) != 4 {
		t.Fatalf("expected 4 messages after 2 turns, got %d", len(conv))
	}
}

func TestAgent_ErrorRollsBackUserMessage(t *testing.T) {
	mock := &mockCompleter{err: errors.New("api error")}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
	})

	err := a.Run(context.Background(), "hello", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(a.Conversation()) != 0 {
		t.Fatalf("expected 0 messages after error, got %d", len(a.Conversation()))
	}
}

func TestAgent_NilHandler(t *testing.T) {
	mock := &mockCompleter{response: "test"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
	})

	if err := a.Run(context.Background(), "hello", nil); err != nil {
		t.Fatalf("Run with nil handler failed: %v", err)
	}
}

func TestAgent_ToolLoop_Happy(t *testing.T) {
	registry := NewToolRegistry()
	var toolCalled bool
	mustRegister(t, registry, Tool{
		Name: "get_time",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			toolCalled = true
			return "13:37", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "toolu_1", Name: "get_time", Input: json.RawMessage(`{}`)}}},
			{Text: "It is 13:37."},
		},
	}
	a := NewAgent(mock, registry, Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
	})

	var received []string
	err := a.Run(context.Background(), "what time?", func(e Event) {
		if e.Type == EventTextDelta {
			received = append(received, e.Text)
		}
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !toolCalled {
		t.Fatal("expected tool to be called")
	}
	if mock.call != 2 {
		t.Fatalf("expected 2 completer calls, got %d", mock.call)
	}

	// Conversation: user, assistant(tool_use), user(tool_result), assistant(text).
	conv := a.Conversation()
	if len(conv) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(conv))
	}

	if len(received) != 1 || received[0] != "It is 13:37." {
		t.Fatalf("expected final text ['It is 13:37.'], got %v", received)
	}
}

func TestAgent_ToolLoop_ParallelBatch(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "a",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "A", nil
		},
	})
	mustRegister(t, registry, Tool{
		Name: "b",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "B", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{
				{ID: "t1", Name: "a", Input: json.RawMessage(`{}`)},
				{ID: "t2", Name: "b", Input: json.RawMessage(`{}`)},
			}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	if err := a.Run(context.Background(), "do both", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Conversation: user, assistant(2 tool_use), user(2 tool_result), assistant(text).
	conv := a.Conversation()
	if len(conv) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(conv))
	}
}

func TestAgent_ToolLoop_MaxIterations(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "loop",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "again", nil
		},
	})

	// Every turn returns a tool_use — loop can only end via MaxIterations.
	responses := make([]scriptedResponse, 5)
	for i := range responses {
		responses[i] = scriptedResponse{
			ToolCalls: []scriptedToolCall{{ID: "t", Name: "loop", Input: json.RawMessage(`{}`)}},
		}
	}
	mock := &mockCompleter{responses: responses}
	a := NewAgent(mock, registry, Config{
		Model:         "claude-sonnet-4-5",
		MaxTokens:     100,
		MaxIterations: 3,
	})

	err := a.Run(context.Background(), "go", nil)
	if err == nil {
		t.Fatal("expected error from MaxIterations breach")
	}
	if !errors.Is(err, ErrMaxIterations) {
		t.Fatalf("expected errors.Is ErrMaxIterations, got %v", err)
	}
	if mock.call != 3 {
		t.Fatalf("expected 3 completer calls (MaxIterations=3), got %d", mock.call)
	}
}

func TestAgent_ToolLoop_UnknownToolContinues(t *testing.T) {
	registry := NewToolRegistry()
	// No tools registered — the LLM's tool call will produce an error
	// tool_result, and the next turn should still run normally.
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "toolu_1", Name: "missing", Input: json.RawMessage(`{}`)}}},
			{Text: "sorry, I cannot do that"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	if err := a.Run(context.Background(), "use missing", nil); err != nil {
		t.Fatalf("Run should not error on unknown tool: %v", err)
	}
	if mock.call != 2 {
		t.Fatalf("expected 2 completer calls, got %d", mock.call)
	}
}

func TestAgent_ToolLoop_MidLoopErrorNoRollback(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "ok",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "fine", nil
		},
	})

	mock := &failAfterNCompleter{
		script: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "toolu_1", Name: "ok", Input: json.RawMessage(`{}`)}}},
		},
		failAfter: 1,
		err:       errors.New("mid-loop api error"),
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	err := a.Run(context.Background(), "start", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	// After turn 0 succeeded and turn 1 failed, the conversation retains:
	// user, assistant(tool_use), user(tool_result). The user message is
	// NOT rolled back because the failure was mid-loop.
	conv := a.Conversation()
	if len(conv) != 3 {
		t.Fatalf("expected 3 messages (no rollback on mid-loop error), got %d", len(conv))
	}
}

func ptrInt64Agent(v int64) *int64    { return &v }
func ptrStringAgent(v string) *string { return &v }

func TestAgent_Thinking_EnabledPassthrough(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	cfg := Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64Agent(2048)},
	}
	a := NewAgent(mock, NewToolRegistry(), cfg)

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(mock.capturedRequests) != 1 {
		t.Fatalf("expected 1 captured request, got %d", len(mock.capturedRequests))
	}
	got := mock.capturedRequests[0].Thinking
	if got == nil || got.Type != "enabled" {
		t.Fatalf("captured Thinking = %+v, want Type=enabled", got)
	}
	if got.BudgetTokens == nil || *got.BudgetTokens != 2048 {
		t.Errorf("captured BudgetTokens = %v, want 2048", got.BudgetTokens)
	}
}

func TestAgent_Thinking_AdaptivePassthrough(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "adaptive"},
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got := mock.capturedRequests[0].Thinking
	if got == nil || got.Type != "adaptive" {
		t.Fatalf("captured Thinking = %+v, want Type=adaptive", got)
	}
}

func TestAgent_Thinking_DisabledPassthrough(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "disabled"},
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got := mock.capturedRequests[0].Thinking
	if got == nil || got.Type != "disabled" {
		t.Fatalf("captured Thinking = %+v, want Type=disabled", got)
	}
}

func TestAgent_Thinking_UnrecognizedTypePassthrough(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "experimental_x"},
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got := mock.capturedRequests[0].Thinking
	if got == nil || got.Type != "experimental_x" {
		t.Fatalf("captured Thinking = %+v, want Type=experimental_x preserved verbatim", got)
	}
}

func TestAgent_Thinking_DisplaySummarizedPreserved(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	display := "summarized"
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64Agent(1024), Display: &display},
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got := mock.capturedRequests[0].Thinking
	if got == nil || got.Display == nil || *got.Display != "summarized" {
		t.Fatalf("captured Display = %+v, want pointer to \"summarized\"", got.Display)
	}
}

func TestAgent_Thinking_OmittedWhenUnconfigured(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if mock.capturedRequests[0].Thinking != nil {
		t.Fatalf("captured Thinking = %+v, want nil", mock.capturedRequests[0].Thinking)
	}
}

func TestAgent_Thinking_MultiTurnPreservation(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "ping",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "pong", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{
				Thinking:  "I should ping",
				Signature: "sig_turn1",
				ToolCalls: []scriptedToolCall{{ID: "toolu_1", Name: "ping", Input: json.RawMessage(`{}`)}},
			},
			{Text: "got pong"},
		},
	}
	a := NewAgent(mock, registry, Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64Agent(2048)},
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(mock.capturedRequests) != 2 {
		t.Fatalf("expected 2 captured requests, got %d", len(mock.capturedRequests))
	}

	// Turn 2's request must contain the prior assistant message with both
	// the thinking block (signature preserved) AND the tool_use block. The
	// thinking block must precede the tool_use block.
	turn2 := mock.capturedRequests[1].Messages
	// Messages: [user, assistant(thinking + tool_use), user(tool_result)]
	if len(turn2) != 3 {
		t.Fatalf("expected 3 messages on turn 2, got %d", len(turn2))
	}
	if turn2[1].Role != anthropic.MessageParamRoleAssistant {
		t.Fatalf("expected turn2[1] to be assistant, got role=%q", turn2[1].Role)
	}
	assistantBlocks := turn2[1].Content
	if len(assistantBlocks) < 2 {
		t.Fatalf("expected at least 2 blocks in assistant message, got %d: %+v", len(assistantBlocks), assistantBlocks)
	}
	// First block must be the thinking block with signature preserved.
	thinkingBlock := assistantBlocks[0].OfThinking
	if thinkingBlock == nil {
		t.Fatalf("expected first block to be thinking, got %+v", assistantBlocks[0])
	}
	if thinkingBlock.Signature != "sig_turn1" {
		t.Errorf("preserved signature = %q, want %q", thinkingBlock.Signature, "sig_turn1")
	}
	if thinkingBlock.Thinking != "I should ping" {
		t.Errorf("preserved thinking text = %q, want %q", thinkingBlock.Thinking, "I should ping")
	}
	// A tool_use block must follow the thinking block.
	if assistantBlocks[1].OfToolUse == nil {
		t.Errorf("expected second block to be tool_use, got %+v", assistantBlocks[1])
	}
}

func TestAgent_Thinking_TextOnlyTurnPreservesBlock(t *testing.T) {
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Thinking: "musing", Signature: "sig_only", Text: "the answer is 42"},
		},
	}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64Agent(2048)},
	})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	conv := a.Conversation()
	if len(conv) != 2 {
		t.Fatalf("expected 2 messages (user + assistant), got %d", len(conv))
	}
	if conv[1].Role != anthropic.MessageParamRoleAssistant {
		t.Fatalf("expected conv[1] to be assistant, got role=%q", conv[1].Role)
	}
	assistantBlocks := conv[1].Content
	if len(assistantBlocks) != 2 {
		t.Fatalf("expected 2 blocks (thinking + text), got %d: %+v", len(assistantBlocks), assistantBlocks)
	}
	thinkingBlock := assistantBlocks[0].OfThinking
	if thinkingBlock == nil {
		t.Fatalf("expected first block to be thinking, got %+v", assistantBlocks[0])
	}
	if thinkingBlock.Signature != "sig_only" {
		t.Errorf("preserved signature = %q, want %q", thinkingBlock.Signature, "sig_only")
	}
}

func TestAgent_Thinking_StreamingDeltasSurfaceThroughHandler(t *testing.T) {
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Thinking: "hmm", Signature: "sig_stream", Text: "done"},
		},
	}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64Agent(1024), Display: ptrStringAgent("summarized")},
	})

	var observed []Event
	err := a.Run(context.Background(), "go", func(e Event) {
		if e.Type != EventDone {
			observed = append(observed, e)
		}
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(observed) != 3 {
		t.Fatalf("expected 3 events (thinking, signature, text), got %d: %+v", len(observed), observed)
	}
	if observed[0].Type != EventThinkingDelta || observed[0].Thinking != "hmm" {
		t.Errorf("event[0] = %+v, want EventThinkingDelta with thinking=\"hmm\"", observed[0])
	}
	if observed[1].Type != EventSignatureDelta || observed[1].Signature != "sig_stream" {
		t.Errorf("event[1] = %+v, want EventSignatureDelta with signature=\"sig_stream\"", observed[1])
	}
	if observed[2].Type != EventTextDelta || observed[2].Text != "done" {
		t.Errorf("event[2] = %+v, want EventTextDelta with text=\"done\"", observed[2])
	}
}

func TestAgent_Effort_Passthrough(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Effort:    ptrStringAgent("low"),
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got := mock.capturedRequests[0].Effort
	if got == nil || *got != "low" {
		t.Fatalf("captured Effort = %v, want pointer to \"low\"", got)
	}
}

func TestAgent_Effort_ArbitraryString(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Effort:    ptrStringAgent("custom"),
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got := mock.capturedRequests[0].Effort
	if got == nil || *got != "custom" {
		t.Fatalf("captured Effort = %v, want pointer to \"custom\" (no validation)", got)
	}
}

func TestAgent_Effort_OmittedWhenUnset(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if mock.capturedRequests[0].Effort != nil {
		t.Fatalf("captured Effort = %+v, want nil", mock.capturedRequests[0].Effort)
	}
}

func TestAgent_Effort_IndependentOfThinking_Enabled(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64Agent(2048)},
		Effort:    ptrStringAgent("high"),
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	req := mock.capturedRequests[0]
	if req.Thinking == nil || req.Thinking.Type != "enabled" {
		t.Errorf("Thinking = %+v, want Type=enabled", req.Thinking)
	}
	if req.Effort == nil || *req.Effort != "high" {
		t.Errorf("Effort = %v, want pointer to \"high\"", req.Effort)
	}
}

func TestAgent_Effort_IndependentOfThinking_Disabled(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "disabled"},
		Effort:    ptrStringAgent("medium"),
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	req := mock.capturedRequests[0]
	if req.Thinking == nil || req.Thinking.Type != "disabled" {
		t.Errorf("Thinking = %+v, want Type=disabled", req.Thinking)
	}
	if req.Effort == nil || *req.Effort != "medium" {
		t.Errorf("Effort = %v, want pointer to \"medium\"", req.Effort)
	}
}

func TestAgent_Effort_IndependentOfThinking_Unset(t *testing.T) {
	mock := &mockCompleter{response: "ok"}
	a := NewAgent(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Effort:    ptrStringAgent("max"),
	})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	req := mock.capturedRequests[0]
	if req.Thinking != nil {
		t.Errorf("Thinking = %+v, want nil", req.Thinking)
	}
	if req.Effort == nil || *req.Effort != "max" {
		t.Errorf("Effort = %v, want pointer to \"max\"", req.Effort)
	}
}

// failAfterNCompleter returns scripted responses for the first N calls,
// then returns err.
type failAfterNCompleter struct {
	script    []scriptedResponse
	failAfter int
	err       error
	call      int
}

func (m *failAfterNCompleter) Complete(_ context.Context, _ CompletionRequest) (*EventStream, error) {
	if m.call >= m.failAfter {
		return nil, m.err
	}
	r := m.script[m.call]
	m.call++

	msg := anthropic.Message{
		ID:    "msg_test",
		Role:  "assistant",
		Model: "claude-sonnet-4-5",
		Usage: anthropic.Usage{InputTokens: 10, OutputTokens: 5},
	}
	texts := []string{}
	if r.Text != "" {
		msg.Content = append(msg.Content, anthropic.ContentBlockUnion{Type: "text", Text: r.Text})
		texts = append(texts, r.Text)
	}
	for _, tc := range r.ToolCalls {
		msg.Content = append(msg.Content, anthropic.ContentBlockUnion{
			Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Input,
		})
	}
	if len(r.ToolCalls) > 0 {
		msg.StopReason = anthropic.StopReasonToolUse
	} else {
		msg.StopReason = anthropic.StopReasonEndTurn
	}
	return NewTestEventStream(texts, msg), nil
}
