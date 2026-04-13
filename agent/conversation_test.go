package agent

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestConversationState_AppendAndMessages(t *testing.T) {
	cs := &ConversationState{}

	if cs.Len() != 0 {
		t.Fatalf("expected len 0, got %d", cs.Len())
	}

	cs.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("hello")))
	cs.Append(anthropic.NewAssistantMessage(anthropic.NewTextBlock("hi there")))

	if cs.Len() != 2 {
		t.Fatalf("expected len 2, got %d", cs.Len())
	}

	msgs := cs.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Verify Messages returns a copy
	msgs = append(msgs, anthropic.NewUserMessage(anthropic.NewTextBlock("extra")))
	if cs.Len() != 2 {
		t.Fatalf("expected original len still 2, got %d", cs.Len())
	}
}

func TestConversationState_Rollback(t *testing.T) {
	cs := &ConversationState{}
	cs.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("msg1")))
	cs.Append(anthropic.NewAssistantMessage(anthropic.NewTextBlock("msg2")))
	cs.Append(anthropic.NewUserMessage(anthropic.NewTextBlock("msg3")))

	cs.Rollback(1)
	if cs.Len() != 2 {
		t.Fatalf("expected len 2 after rollback(1), got %d", cs.Len())
	}

	cs.Rollback(5) // more than available
	if cs.Len() != 0 {
		t.Fatalf("expected len 0 after rollback(5), got %d", cs.Len())
	}
}
