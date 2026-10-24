package agent

import "github.com/anthropics/anthropic-sdk-go/v2"

// Role names as reported by messageRole.
const (
	roleUser      = "user"
	roleAssistant = "assistant"
)

// messageRole returns the role of m, or "" when no role variant is set.
func messageRole(m anthropic.MessageParam) string {
	if r := m.GetRole(); r != nil {
		return *r
	}
	return ""
}

// messageContent returns the content blocks of a user or assistant
// message. System messages carry text blocks only and are not part of a
// conversation history, so they and unset messages yield nil.
func messageContent(m anthropic.MessageParam) []anthropic.ContentBlockParamUnion {
	switch {
	case m.OfUser != nil:
		return m.OfUser.Content
	case m.OfAssistant != nil:
		return m.OfAssistant.Content
	}
	return nil
}

// withContent returns a copy of m carrying blocks in place of its content.
// The role variant is reallocated rather than mutated, because copies of
// a MessageParam share the variant they point to.
func withContent(m anthropic.MessageParam, blocks []anthropic.ContentBlockParamUnion) anthropic.MessageParam {
	switch {
	case m.OfUser != nil:
		return anthropic.NewUserMessage(blocks...)
	case m.OfAssistant != nil:
		return anthropic.NewAssistantMessage(blocks...)
	}
	return m
}
