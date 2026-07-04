package agent

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// HistoryValidationError is returned by NewAgentWithHistory when the
// supplied history violates one of the five S2.15 resumption invariants:
//
//  1. The history is empty, or ends with an assistant message.
//  2. User and assistant messages alternate, starting with a user message.
//  3. Every assistant tool_use block has a matching tool_result block (by
//     tool-use ID) in the immediately following user message.
//  4. No tool_result block appears without a matching tool_use for the
//     same ID in the immediately preceding assistant message.
//  5. If the history ends with an assistant message, that message
//     contains no tool_use blocks.
type HistoryValidationError struct {
	// Rule is the violated invariant, 1 through 5.
	Rule int
	// MessageIndex is the index of the offending message in the input.
	MessageIndex int
	// ToolUseID is the offending tool-use ID for rules 3 and 4; empty
	// otherwise.
	ToolUseID string
	// Detail is a human-readable explanation of the violation.
	Detail string
}

func (e *HistoryValidationError) Error() string {
	return fmt.Sprintf("history validation failed (rule %d) at message %d: %s",
		e.Rule, e.MessageIndex, e.Detail)
}

// NewAgentWithHistory creates an Agent whose conversation state is
// initialized to a pre-existing message history, so a conversation
// persisted across process boundaries can be resumed (S2.15). The history
// must be in the SDK-native representation returned by Conversation.
//
// The history is validated against the five S2.15 resumption invariants;
// on violation the constructor returns a *HistoryValidationError and no
// Agent. An empty (or nil) history is valid and equivalent to NewAgent.
//
// Resumption is for top-level Agents only: sub-agents retain live
// instances within a parent run (S2.11) and are not constructed from
// prior history.
func NewAgentWithHistory(completer Completer, registry *ToolRegistry, config Config, history []anthropic.MessageParam) (*Agent, error) {
	if err := validateHistory(history); err != nil {
		return nil, err
	}
	msgs := make([]anthropic.MessageParam, len(history))
	copy(msgs, history)
	a := NewAgent(completer, registry, config)
	a.conversation = ConversationState{messages: msgs}
	return a, nil
}

// validateHistory checks the five S2.15 resumption invariants. Message
// ordering (rules 1 and 2) is checked before tool-use pairing (rules 3,
// 4, and 5) so that a history violating both reports the ordering rule.
// Only message structure is validated: thinking, text, and other content
// blocks pass through untouched, and the history need not have been
// produced by this library.
func validateHistory(history []anthropic.MessageParam) error {
	if len(history) == 0 {
		return nil
	}

	// Rules 1 and 2: user/assistant alternation from a user start, ending
	// with an assistant message.
	for i, msg := range history {
		switch msg.Role {
		case anthropic.MessageParamRoleUser, anthropic.MessageParamRoleAssistant:
		default:
			return &HistoryValidationError{
				Rule: 2, MessageIndex: i,
				Detail: fmt.Sprintf("role %q is neither user nor assistant", msg.Role),
			}
		}
		if i == 0 && msg.Role != anthropic.MessageParamRoleUser {
			return &HistoryValidationError{
				Rule: 2, MessageIndex: i,
				Detail: "history must start with a user message",
			}
		}
		if i > 0 && msg.Role == history[i-1].Role {
			return &HistoryValidationError{
				Rule: 2, MessageIndex: i,
				Detail: fmt.Sprintf("consecutive %s messages break alternation", msg.Role),
			}
		}
	}
	if last := history[len(history)-1]; last.Role != anthropic.MessageParamRoleAssistant {
		return &HistoryValidationError{
			Rule: 1, MessageIndex: len(history) - 1,
			Detail: "history must end with an assistant message",
		}
	}

	// Rules 3, 4, and 5: tool_use/tool_result pairing across adjacent
	// messages. Alternation is already established, so the message after
	// an assistant message is a user message and vice versa.
	for i, msg := range history {
		if msg.Role == anthropic.MessageParamRoleAssistant {
			uses := toolUseIDs(msg)
			if i == len(history)-1 {
				if len(uses) > 0 {
					return &HistoryValidationError{
						Rule: 5, MessageIndex: i, ToolUseID: uses[0],
						Detail: "trailing assistant message contains unresolved tool_use blocks",
					}
				}
				continue
			}
			results := toolResultIDSet(history[i+1])
			for _, id := range uses {
				if !results[id] {
					return &HistoryValidationError{
						Rule: 3, MessageIndex: i, ToolUseID: id,
						Detail: fmt.Sprintf("tool_use %q has no tool_result in the following user message", id),
					}
				}
			}
			continue
		}
		var prevUses map[string]bool
		if i > 0 {
			prevUses = toolUseIDSet(history[i-1])
		}
		for _, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			if id := block.OfToolResult.ToolUseID; !prevUses[id] {
				return &HistoryValidationError{
					Rule: 4, MessageIndex: i, ToolUseID: id,
					Detail: fmt.Sprintf("tool_result %q has no tool_use in the preceding assistant message", id),
				}
			}
		}
	}
	return nil
}

// toolUseIDs returns the IDs of all tool_use blocks in msg, in order.
func toolUseIDs(msg anthropic.MessageParam) []string {
	var ids []string
	for _, block := range msg.Content {
		if block.OfToolUse != nil {
			ids = append(ids, block.OfToolUse.ID)
		}
	}
	return ids
}

func toolUseIDSet(msg anthropic.MessageParam) map[string]bool {
	set := make(map[string]bool)
	for _, id := range toolUseIDs(msg) {
		set[id] = true
	}
	return set
}

// toolResultIDSet returns the tool-use IDs answered by tool_result blocks
// in msg.
func toolResultIDSet(msg anthropic.MessageParam) map[string]bool {
	set := make(map[string]bool)
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			set[block.OfToolResult.ToolUseID] = true
		}
	}
	return set
}
