package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

const subTestModel = anthropic.Model("claude-sonnet-4-5")

// callSubAgent invokes a sub-agent tool's Execute directly with a top-level
// runtime (depth 0) on the context, as Agent.Run would establish it. handler
// and approval emulate the propagated parent stream and approval gate.
func callSubAgent(t *testing.T, tool Tool, args string, handler EventHandler, approval ApprovalCallback) (string, error) {
	t.Helper()
	ctx := withSubAgentRuntime(context.Background(), subAgentRuntime{
		parentHandler: handler,
		approval:      approval,
		depth:         0,
	})
	return tool.Execute(ctx, json.RawMessage(args))
}

func TestSubAgent_OneShot_ReturnsFinalMessage(t *testing.T) {
	child := &mockCompleter{response: "the sub-agent answer"}
	tool, err := NewSubAgentTool(child, SubAgentDefinition{
		Name:        "researcher",
		Description: "Researches things.",
		System:      "You research.",
		Model:       subTestModel,
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool: %v", err)
	}

	got, err := callSubAgent(t, tool, `{"prompt":"find X"}`, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "the sub-agent answer" {
		t.Fatalf("result = %q, want %q", got, "the sub-agent answer")
	}
	if child.call == 0 && len(child.capturedRequests) == 0 {
		t.Fatal("expected the sub-agent to call its completer")
	}
	// One-shot leaves no session handle in the result.
	if strings.Contains(got, "sessionId:") {
		t.Errorf("one-shot result should not carry a sessionId: %q", got)
	}
}

func TestSubAgent_OneShot_StartsFreshEachCall(t *testing.T) {
	child := &mockCompleter{response: "ok"}
	tool, err := NewSubAgentTool(child, SubAgentDefinition{
		Name: "fresh", Description: "d", Model: subTestModel,
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool: %v", err)
	}

	if _, err := callSubAgent(t, tool, `{"prompt":"first"}`, nil, nil); err != nil {
		t.Fatalf("Execute 1: %v", err)
	}
	if _, err := callSubAgent(t, tool, `{"prompt":"second"}`, nil, nil); err != nil {
		t.Fatalf("Execute 2: %v", err)
	}
	// Each fresh sub-agent sees only its own single user message — no carry-over.
	for i, req := range child.capturedRequests {
		if len(req.Messages) != 1 {
			t.Fatalf("call %d: expected 1 message (fresh start), got %d", i, len(req.Messages))
		}
	}
}

func TestSubAgent_MultiTurn_ResumesWithHistory(t *testing.T) {
	child := &mockCompleter{
		responses: []scriptedResponse{
			{Text: "first reply"},
			{Text: "second reply"},
		},
	}
	tool, err := NewSubAgentTool(child, SubAgentDefinition{
		Name: "chat", Description: "d", Model: subTestModel, MultiTurn: true,
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool: %v", err)
	}

	first, err := callSubAgent(t, tool, `{"prompt":"hello"}`, nil, nil)
	if err != nil {
		t.Fatalf("Execute 1: %v", err)
	}
	id := parseSessionID(t, first)
	if id == "" {
		t.Fatalf("first result missing sessionId: %q", first)
	}

	second, err := callSubAgent(t, tool, `{"prompt":"again","session_id":"`+id+`"}`, nil, nil)
	if err != nil {
		t.Fatalf("Execute 2: %v", err)
	}
	if !strings.HasPrefix(second, "second reply") {
		t.Errorf("second result = %q, want it to start with %q", second, "second reply")
	}
	if got := parseSessionID(t, second); got != id {
		t.Errorf("resumed sessionId = %q, want %q (same instance)", got, id)
	}

	// The resumed run must carry the prior turn's history: user, assistant,
	// user — three messages on the second completer call.
	if len(child.capturedRequests) != 2 {
		t.Fatalf("expected 2 completer calls, got %d", len(child.capturedRequests))
	}
	resumeReq := child.capturedRequests[1]
	if len(resumeReq.Messages) != 3 {
		t.Fatalf("resumed request: expected 3 messages (history intact), got %d", len(resumeReq.Messages))
	}
}

func TestSubAgent_NestingRejected(t *testing.T) {
	child := &mockCompleter{response: "should not run"}
	tool, err := NewSubAgentTool(child, SubAgentDefinition{
		Name: "deep", Description: "d", Model: subTestModel,
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool: %v", err)
	}

	// Simulate invocation from within a sub-agent (depth 1).
	ctx := withSubAgentRuntime(context.Background(), subAgentRuntime{depth: 1})
	_, err = tool.Execute(ctx, json.RawMessage(`{"prompt":"go deeper"}`))
	if err == nil {
		t.Fatal("expected nesting to be rejected at depth 1")
	}
	if !strings.Contains(err.Error(), "nesting") {
		t.Errorf("error = %v, want a nesting-depth error", err)
	}
	if len(child.capturedRequests) != 0 {
		t.Error("rejected sub-agent must not run its completer")
	}
}

func TestSubAgent_HITLPropagation(t *testing.T) {
	// Child registry has a HITL-flagged tool. A placeholder approval is set so
	// registration succeeds; NewSubAgentTool replaces it with the propagation
	// bridge.
	childReg := NewToolRegistry()
	childReg.SetApprovalCallback(func(context.Context, ToolCall) (bool, error) {
		t.Error("placeholder approval should be replaced by the bridge")
		return false, nil
	})
	var hitlExecuted bool
	mustRegister(t, childReg, Tool{
		Name: "danger",
		HITL: true,
		Execute: func(context.Context, json.RawMessage) (string, error) {
			hitlExecuted = true
			return "did it", nil
		},
	})

	child := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "danger", Input: json.RawMessage(`{}`)}}},
			{Text: "all done"},
		},
	}
	tool, err := NewSubAgentTool(child, SubAgentDefinition{
		Name: "agent", Description: "d", Model: subTestModel, Tools: childReg,
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool: %v", err)
	}

	t.Run("ParentApprovalGovernsAndIdentifiesSubAgent", func(t *testing.T) {
		hitlExecuted = false
		var sawSubAgentDepth int
		var approvalCalls int
		parentApproval := func(ctx context.Context, call ToolCall) (bool, error) {
			approvalCalls++
			_, _, depth := SubAgentContext(ctx)
			sawSubAgentDepth = depth
			return true, nil
		}
		got, err := callSubAgent(t, tool, `{"prompt":"do danger"}`, nil, parentApproval)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if approvalCalls != 1 {
			t.Errorf("parent approval calls = %d, want 1", approvalCalls)
		}
		if sawSubAgentDepth != 1 {
			t.Errorf("approval saw depth %d, want 1 (sub-agent call)", sawSubAgentDepth)
		}
		if !hitlExecuted {
			t.Error("approved HITL tool should execute")
		}
		if got != "all done" {
			t.Errorf("result = %q, want %q", got, "all done")
		}
	})

	t.Run("DenialSurfacesAsErrorInsideSubAgent", func(t *testing.T) {
		// Reset the child script for a second independent run.
		child.responses = []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "danger", Input: json.RawMessage(`{}`)}}},
			{Text: "could not, was denied"},
		}
		child.call = 0
		child.capturedRequests = nil
		hitlExecuted = false

		deny := func(context.Context, ToolCall) (bool, error) { return false, nil }
		got, err := callSubAgent(t, tool, `{"prompt":"do danger"}`, nil, deny)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if hitlExecuted {
			t.Error("denied HITL tool must not execute")
		}
		// The denial becomes an error tool_result fed back into the sub-agent's
		// own loop, which then produces its final message.
		if got != "could not, was denied" {
			t.Errorf("result = %q, want the sub-agent's post-denial message", got)
		}
	})
}

func TestSubAgent_StreamIsolatedByDefault(t *testing.T) {
	child := &mockCompleter{response: "child says hi"}
	tool, err := NewSubAgentTool(child, SubAgentDefinition{
		Name: "quiet", Description: "d", Model: subTestModel, // Forward defaults false
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool: %v", err)
	}

	var parentEvents []Event
	parentHandler := func(e Event) { parentEvents = append(parentEvents, e) }
	if _, err := callSubAgent(t, tool, `{"prompt":"hi"}`, parentHandler, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, e := range parentEvents {
		if e.AgentName != "" || e.Depth != 0 {
			t.Errorf("isolated sub-agent leaked event to parent: %+v", e)
		}
	}
}

func TestSubAgent_StreamForwardingAttributed(t *testing.T) {
	child := &mockCompleter{response: "child says hi"}
	tool, err := NewSubAgentTool(child, SubAgentDefinition{
		Name: "loud", Description: "d", Model: subTestModel, Forward: true,
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool: %v", err)
	}

	var got []Event
	parentHandler := func(e Event) { got = append(got, e) }
	if _, err := callSubAgent(t, tool, `{"prompt":"hi"}`, parentHandler, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var sawTaggedText bool
	for _, e := range got {
		if e.Type == EventTextDelta && e.Text == "child says hi" {
			sawTaggedText = true
			if e.AgentName != "loud" {
				t.Errorf("forwarded event AgentName = %q, want %q", e.AgentName, "loud")
			}
			if e.Depth != 1 {
				t.Errorf("forwarded event Depth = %d, want 1", e.Depth)
			}
		}
	}
	if !sawTaggedText {
		t.Fatalf("expected the forwarded child text event, got %+v", got)
	}
}

// TestSubAgent_ParallelForwardingNoRace runs two sub-agent tools concurrently,
// both forwarding to a shared (serialized) parent handler, under -race.
func TestSubAgent_ParallelForwardingNoRace(t *testing.T) {
	mk := func(name string) Tool {
		tool, err := NewSubAgentTool(&mockCompleter{response: name + " output"}, SubAgentDefinition{
			Name: name, Description: "d", Model: subTestModel, Forward: true,
		})
		if err != nil {
			t.Fatalf("NewSubAgentTool: %v", err)
		}
		return tool
	}
	toolA, toolB := mk("a"), mk("b")

	// Emulate the serialized parent handler Agent.Run installs.
	var mu sync.Mutex
	var count int
	raw := func(Event) { count++ } // unsynchronized; the serializer must protect it
	serialized := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		raw(e)
	}
	ctx := withSubAgentRuntime(context.Background(), subAgentRuntime{
		parentHandler: serialized, depth: 0,
	})

	var wg sync.WaitGroup
	for _, tl := range []Tool{toolA, toolB} {
		wg.Add(1)
		go func(tool Tool) {
			defer wg.Done()
			if _, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"go"}`)); err != nil {
				t.Errorf("Execute: %v", err)
			}
		}(tl)
	}
	wg.Wait()
	if count == 0 {
		t.Fatal("expected forwarded events from the parallel sub-agents")
	}
}

func TestNewSubAgentTool_Validation(t *testing.T) {
	good := &mockCompleter{response: "x"}
	cases := []struct {
		name      string
		completer Completer
		def       SubAgentDefinition
	}{
		{"nil completer", nil, SubAgentDefinition{Name: "n", Model: subTestModel}},
		{"empty name", good, SubAgentDefinition{Model: subTestModel}},
		{"empty model", good, SubAgentDefinition{Name: "n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewSubAgentTool(tc.completer, tc.def); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestSubAgent_MultiTurn_DocumentsSessionConvention(t *testing.T) {
	completer := &mockCompleter{response: "x"}

	multi, err := NewSubAgentTool(completer, SubAgentDefinition{
		Name: "chat", Description: "Chats with the user.", Model: subTestModel, MultiTurn: true,
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool (multi-turn): %v", err)
	}
	if !strings.Contains(multi.Description, "Chats with the user.") {
		t.Errorf("multi-turn description dropped the caller's text: %q", multi.Description)
	}
	if !strings.Contains(multi.Description, sessionIDPrefix+"<id>") {
		t.Errorf("multi-turn description does not document the %q<id> output convention: %q",
			sessionIDPrefix, multi.Description)
	}
	// The session_id input param should also point at the trailer.
	sid, ok := multi.InputSchema.Properties.(map[string]any)["session_id"].(map[string]any)
	if !ok {
		t.Fatalf("multi-turn tool missing session_id property: %+v", multi.InputSchema.Properties)
	}
	if desc, _ := sid["description"].(string); !strings.Contains(desc, sessionIDPrefix+"<id>") {
		t.Errorf("session_id param does not reference the %q<id> line: %q", sessionIDPrefix, desc)
	}

	oneShot, err := NewSubAgentTool(completer, SubAgentDefinition{
		Name: "tool", Description: "Does a thing.", Model: subTestModel,
	})
	if err != nil {
		t.Fatalf("NewSubAgentTool (one-shot): %v", err)
	}
	if oneShot.Description != "Does a thing." {
		t.Errorf("one-shot description should be verbatim (no trailer docs), got %q", oneShot.Description)
	}
}

func parseSessionID(t *testing.T, result string) string {
	t.Helper()
	const marker = "sessionId: "
	i := strings.LastIndex(result, marker)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(result[i+len(marker):])
}
