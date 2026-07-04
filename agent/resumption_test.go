package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// marshalMessages renders a history as JSON for whole-history equality
// assertions, the same round-trip technique buildStream relies on.
func marshalMessages(t *testing.T, msgs []anthropic.MessageParam) string {
	t.Helper()
	data, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	return string(data)
}

func userText(text string) anthropic.MessageParam {
	return anthropic.NewUserMessage(anthropic.NewTextBlock(text))
}

func assistantText(text string) anthropic.MessageParam {
	return anthropic.NewAssistantMessage(anthropic.NewTextBlock(text))
}

// S6.24: an Agent constructed with a valid prior history initializes its
// conversation state to that history, and a subsequent run extends from it —
// the API request includes the prior messages.
func TestNewAgentWithHistory_SeedsConversationAndRunExtends(t *testing.T) {
	history := []anthropic.MessageParam{
		userText("what is the capital of France?"),
		assistantText("Paris."),
	}

	mock := &mockCompleter{response: "You asked about the capital of France."}
	a, err := NewAgentWithHistory(mock, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		// Caching breakpoints rewrite cache_control on the request copy
		// (S6.26 covers that); disable so the request prefix is
		// byte-comparable to the seeded history.
		DisablePromptCaching: true,
	}, history)
	if err != nil {
		t.Fatalf("NewAgentWithHistory failed: %v", err)
	}

	if got, want := marshalMessages(t, a.Conversation()), marshalMessages(t, history); got != want {
		t.Fatalf("seeded conversation = %s, want %s", got, want)
	}

	if err := a.Run(context.Background(), "what did I just ask?", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	req := mock.capturedRequests[0].Messages
	if len(req) != 3 {
		t.Fatalf("expected 3 messages in request (2 seeded + 1 new user), got %d", len(req))
	}
	if got, want := marshalMessages(t, req[:2]), marshalMessages(t, history); got != want {
		t.Fatalf("request prefix = %s, want seeded history %s", got, want)
	}
	if req[2].Role != anthropic.MessageParamRoleUser {
		t.Fatalf("expected request to end with the new user message, got role=%q", req[2].Role)
	}

	// The run extends the seeded history: user + assistant appended.
	if got := a.Conversation(); len(got) != 4 {
		t.Fatalf("expected 4 messages after run, got %d", len(got))
	}
}

// S6.24: an empty history is accepted and yields an Agent equivalent to one
// created via the basic constructor.
func TestNewAgentWithHistory_EmptyHistoryEquivalentToNewAgent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		history []anthropic.MessageParam
	}{
		{"nil", nil},
		{"empty", []anthropic.MessageParam{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockA := &mockCompleter{response: "hello"}
			mockB := &mockCompleter{response: "hello"}
			cfg := Config{Model: "claude-sonnet-4-5", MaxTokens: 100}

			base := NewAgent(mockA, NewToolRegistry(), cfg)
			resumed, err := NewAgentWithHistory(mockB, NewToolRegistry(), cfg, tc.history)
			if err != nil {
				t.Fatalf("NewAgentWithHistory failed: %v", err)
			}
			if n := len(resumed.Conversation()); n != 0 {
				t.Fatalf("expected empty conversation, got %d messages", n)
			}

			if err := base.Run(context.Background(), "hi", nil); err != nil {
				t.Fatalf("base Run failed: %v", err)
			}
			if err := resumed.Run(context.Background(), "hi", nil); err != nil {
				t.Fatalf("resumed Run failed: %v", err)
			}
			if got, want := marshalMessages(t, resumed.Conversation()), marshalMessages(t, base.Conversation()); got != want {
				t.Fatalf("conversation after run = %s, want %s", got, want)
			}
		})
	}
}

// S6.24: round-trip through the read interface for a text-only history —
// messages(B) = messages(A).
func TestNewAgentWithHistory_RoundTripTextOnly(t *testing.T) {
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Text: "first answer"},
			{Text: "second answer"},
		},
	}
	cfg := Config{Model: "claude-sonnet-4-5", MaxTokens: 100}
	a := NewAgent(mock, NewToolRegistry(), cfg)
	if err := a.Run(context.Background(), "first question", nil); err != nil {
		t.Fatalf("Run 1 failed: %v", err)
	}
	if err := a.Run(context.Background(), "second question", nil); err != nil {
		t.Fatalf("Run 2 failed: %v", err)
	}

	b, err := NewAgentWithHistory(&mockCompleter{}, NewToolRegistry(), cfg, a.Conversation())
	if err != nil {
		t.Fatalf("NewAgentWithHistory failed: %v", err)
	}
	if got, want := marshalMessages(t, b.Conversation()), marshalMessages(t, a.Conversation()); got != want {
		t.Fatalf("messages(B) = %s, want messages(A) = %s", got, want)
	}
}

// S6.24: round-trip through the read interface for a history including a
// tool-use turn.
func TestNewAgentWithHistory_RoundTripToolUse(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "get_time",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "13:37", nil
		},
	})
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "toolu_1", Name: "get_time", Input: json.RawMessage(`{}`)}}},
			{Text: "It is 13:37."},
		},
	}
	cfg := Config{Model: "claude-sonnet-4-5", MaxTokens: 100}
	a := NewAgent(mock, registry, cfg)
	if err := a.Run(context.Background(), "what time?", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// Conversation: user, assistant(tool_use), user(tool_result), assistant(text).
	if n := len(a.Conversation()); n != 4 {
		t.Fatalf("expected 4 messages, got %d", n)
	}

	b, err := NewAgentWithHistory(&mockCompleter{}, NewToolRegistry(), cfg, a.Conversation())
	if err != nil {
		t.Fatalf("NewAgentWithHistory failed: %v", err)
	}
	if got, want := marshalMessages(t, b.Conversation()), marshalMessages(t, a.Conversation()); got != want {
		t.Fatalf("messages(B) = %s, want messages(A) = %s", got, want)
	}
}

// S6.24: thinking-bearing turn round-trip — construction succeeds with
// thinking blocks and signatures intact, and the next Completer request
// carries the thinking block ahead of later content, signature unchanged.
func TestNewAgentWithHistory_RoundTripThinkingSignatures(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "ping",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "pong", nil
		},
	})
	mockA := &mockCompleter{
		responses: []scriptedResponse{
			{
				Thinking:  "I should ping",
				Signature: "sig_resume",
				ToolCalls: []scriptedToolCall{{ID: "toolu_1", Name: "ping", Input: json.RawMessage(`{}`)}},
			},
			{Text: "got pong"},
		},
	}
	cfg := Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
		Thinking:  &ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64Agent(2048)},
	}
	a := NewAgent(mockA, registry, cfg)
	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	mockB := &mockCompleter{response: "resumed"}
	b, err := NewAgentWithHistory(mockB, registry, cfg, a.Conversation())
	if err != nil {
		t.Fatalf("NewAgentWithHistory failed: %v", err)
	}
	if got, want := marshalMessages(t, b.Conversation()), marshalMessages(t, a.Conversation()); got != want {
		t.Fatalf("messages(B) = %s, want messages(A) = %s", got, want)
	}

	if err := b.Run(context.Background(), "again", nil); err != nil {
		t.Fatalf("resumed Run failed: %v", err)
	}
	// Request: [user, assistant(thinking + tool_use), user(tool_result),
	// assistant(text), user(new)]. The thinking block must survive with its
	// signature, ahead of the tool_use it accompanied.
	req := mockB.capturedRequests[0].Messages
	if len(req) != 5 {
		t.Fatalf("expected 5 messages in resumed request, got %d", len(req))
	}
	blocks := req[1].Content
	if len(blocks) < 2 || blocks[0].OfThinking == nil {
		t.Fatalf("expected assistant message to lead with a thinking block, got %+v", blocks)
	}
	if blocks[0].OfThinking.Signature != "sig_resume" {
		t.Errorf("signature = %q, want %q", blocks[0].OfThinking.Signature, "sig_resume")
	}
	if blocks[1].OfToolUse == nil {
		t.Errorf("expected tool_use block after thinking, got %+v", blocks[1])
	}
}

// S6.24: each malformed history yields a constructor error identifying the
// violated rule, and no Agent is returned.
func TestNewAgentWithHistory_RejectsMalformedHistory(t *testing.T) {
	toolUse := anthropic.NewToolUseBlock("toolu_1", map[string]any{}, "get_time")
	toolUse2 := anthropic.NewToolUseBlock("toolu_2", map[string]any{}, "get_time")

	cases := []struct {
		name     string
		history  []anthropic.MessageParam
		wantRule int
	}{
		{
			name: "ends with user message",
			history: []anthropic.MessageParam{
				userText("q"), assistantText("a"), userText("q2"),
			},
			wantRule: 1,
		},
		{
			name: "begins with assistant message",
			history: []anthropic.MessageParam{
				assistantText("a"),
			},
			wantRule: 2,
		},
		{
			name: "consecutive same-role messages",
			history: []anthropic.MessageParam{
				userText("q"), userText("q again"), assistantText("a"),
			},
			wantRule: 2,
		},
		{
			name: "non-user/assistant role",
			history: []anthropic.MessageParam{
				{Role: "system", Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("s")}},
				assistantText("a"),
			},
			wantRule: 2,
		},
		{
			name: "tool_use without matching tool_result in next message",
			history: []anthropic.MessageParam{
				userText("q"),
				anthropic.NewAssistantMessage(toolUse, toolUse2),
				anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_1", "13:37", false)),
				assistantText("done"),
			},
			wantRule: 3,
		},
		{
			name: "tool_result without preceding tool_use",
			history: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_orphan", "x", false)),
				assistantText("a"),
			},
			wantRule: 4,
		},
		{
			name: "tool_result answering an earlier, not immediately preceding, tool_use",
			history: []anthropic.MessageParam{
				userText("q"),
				anthropic.NewAssistantMessage(toolUse),
				anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_1", "13:37", false)),
				assistantText("interlude"),
				anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_1", "13:37", false)),
				assistantText("done"),
			},
			wantRule: 4,
		},
		{
			name: "trailing assistant message with tool_use",
			history: []anthropic.MessageParam{
				userText("q"),
				anthropic.NewAssistantMessage(toolUse),
			},
			wantRule: 5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := NewAgentWithHistory(&mockCompleter{}, NewToolRegistry(), Config{
				Model:     "claude-sonnet-4-5",
				MaxTokens: 100,
			}, tc.history)
			if a != nil {
				t.Fatal("expected nil Agent for malformed history")
			}
			var hve *HistoryValidationError
			if !errors.As(err, &hve) {
				t.Fatalf("expected *HistoryValidationError, got %v", err)
			}
			if hve.Rule != tc.wantRule {
				t.Fatalf("Rule = %d, want %d (error: %v)", hve.Rule, tc.wantRule, hve)
			}
		})
	}
}

// Histories that exercise structural edge cases but satisfy all five
// invariants must be accepted.
func TestNewAgentWithHistory_AcceptsValidEdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		history []anthropic.MessageParam
	}{
		{
			name: "tool_result and text in the same user message",
			history: []anthropic.MessageParam{
				userText("q"),
				anthropic.NewAssistantMessage(anthropic.NewToolUseBlock("toolu_1", map[string]any{}, "get_time")),
				anthropic.NewUserMessage(
					anthropic.NewToolResultBlock("toolu_1", "13:37", false),
					anthropic.NewTextBlock("thanks"),
				),
				assistantText("done"),
			},
		},
		{
			name: "trailing assistant message with thinking-only content",
			history: []anthropic.MessageParam{
				userText("q"),
				anthropic.NewAssistantMessage(
					anthropic.NewThinkingBlock("sig_edge", "pondering"),
					anthropic.NewTextBlock("a"),
				),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := NewAgentWithHistory(&mockCompleter{}, NewToolRegistry(), Config{
				Model:     "claude-sonnet-4-5",
				MaxTokens: 100,
			}, tc.history)
			if err != nil {
				t.Fatalf("expected valid history, got error: %v", err)
			}
			if got, want := marshalMessages(t, a.Conversation()), marshalMessages(t, tc.history); got != want {
				t.Fatalf("seeded conversation = %s, want %s", got, want)
			}
		})
	}
}

// The constructor copies the supplied history: later mutation of the
// caller's slice must not affect the Agent's conversation state.
func TestNewAgentWithHistory_DefensiveCopy(t *testing.T) {
	history := []anthropic.MessageParam{
		userText("q"),
		assistantText("a"),
	}
	a, err := NewAgentWithHistory(&mockCompleter{}, NewToolRegistry(), Config{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
	}, history)
	if err != nil {
		t.Fatalf("NewAgentWithHistory failed: %v", err)
	}

	want := marshalMessages(t, a.Conversation())
	history[1] = assistantText("tampered")
	if got := marshalMessages(t, a.Conversation()); got != want {
		t.Fatalf("conversation changed after caller mutation: %s, want %s", got, want)
	}
}
