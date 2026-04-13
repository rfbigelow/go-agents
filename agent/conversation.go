package agent

import "github.com/anthropics/anthropic-sdk-go"

// ConversationState manages the message history for an agent session.
// It stores user messages, assistant responses, and tool results,
// enforcing correct message ordering.
type ConversationState struct {
	messages []anthropic.MessageParam
}

// Append adds a message to the conversation history.
func (cs *ConversationState) Append(msg anthropic.MessageParam) {
	cs.messages = append(cs.messages, msg)
}

// Messages returns a copy of the conversation history.
func (cs *ConversationState) Messages() []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, len(cs.messages))
	copy(out, cs.messages)
	return out
}

// Len returns the number of messages in the conversation.
func (cs *ConversationState) Len() int {
	return len(cs.messages)
}

// Rollback removes the last n messages from the conversation.
// If n exceeds the number of messages, all messages are removed.
func (cs *ConversationState) Rollback(n int) {
	if n >= len(cs.messages) {
		cs.messages = cs.messages[:0]
		return
	}
	cs.messages = cs.messages[:len(cs.messages)-n]
}
