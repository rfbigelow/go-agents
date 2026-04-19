package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// scriptedResponse describes one Completer.Complete result. For the
// default case (plain text end_turn), set Text. For a tool_use turn,
// leave Text empty and populate ToolCalls.
type scriptedResponse struct {
	Text      string
	ToolCalls []scriptedToolCall
}

type scriptedToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// mockCompleter implements Completer for testing. In its simplest form
// (Response set) it returns a single end_turn response on every call — the
// shape M1 tests depend on. Set Responses for a multi-turn script; each
// Complete call consumes one entry in order.
type mockCompleter struct {
	response  string
	err       error
	responses []scriptedResponse
	call      int
}

func (m *mockCompleter) Complete(_ context.Context, _ CompletionRequest) (*EventStream, error) {
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

	texts := []string{}
	if r.Text != "" {
		msg.Content = append(msg.Content, anthropic.ContentBlockUnion{
			Type: "text",
			Text: r.Text,
		})
		texts = append(texts, r.Text)
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

	return NewTestEventStream(texts, msg)
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
