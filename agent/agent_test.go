package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// mockCompleter implements Completer for testing.
type mockCompleter struct {
	response string
	err      error
}

func (m *mockCompleter) Complete(_ context.Context, _ CompletionRequest) (*EventStream, error) {
	if m.err != nil {
		return nil, m.err
	}

	msg := anthropic.Message{
		ID:         "msg_test",
		Role:       "assistant",
		Model:      "claude-sonnet-4-5",
		StopReason: anthropic.StopReasonEndTurn,
		Content: []anthropic.ContentBlockUnion{
			{Type: "text", Text: m.response},
		},
		Usage: anthropic.Usage{InputTokens: 10, OutputTokens: 5},
	}

	return NewTestEventStream([]string{m.response}, msg), nil
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
