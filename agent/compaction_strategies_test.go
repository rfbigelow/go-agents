package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// toolTurnHistory seeds a committed history of three fresh turns where
// the first turn is a tool-use exchange:
//
//	0 user "u1"  (fresh turn 1)
//	1 assistant tool_use
//	2 user tool_result
//	3 assistant "a1 final"
//	4 user "u2"  (fresh turn 2)
//	5 assistant "a2"
//	6 user "u3"  (fresh turn 3)
//	7 assistant "a3"
func toolTurnHistory() []anthropic.MessageParam {
	return []anthropic.MessageParam{
		userText("u1"),
		anthropic.NewAssistantMessage(anthropic.NewToolUseBlock("toolu_1", map[string]any{}, "get_time")),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_1", "13:37", false)),
		assistantText("a1 final"),
		userText("u2"),
		assistantText("a2"),
		userText("u3"),
		assistantText("a3"),
	}
}

func TestTurnStarts(t *testing.T) {
	starts := turnStarts(toolTurnHistory())
	want := []int{0, 4, 6}
	if len(starts) != len(want) {
		t.Fatalf("turnStarts = %v, want %v", starts, want)
	}
	for i := range want {
		if starts[i] != want[i] {
			t.Fatalf("turnStarts = %v, want %v", starts, want)
		}
	}
}

// S6.36: the sliding-window strategy cuts on safe boundaries — a
// tool-use turn is dropped or retained whole — and makes no Completer
// call.
func TestSlidingWindow_SafeBoundariesNoCompleter(t *testing.T) {
	history := toolTurnHistory()
	s := &SlidingWindowStrategy{KeepTurns: 2}

	// Completer deliberately nil: the strategy must not need one.
	res, err := s.Compact(context.Background(), CompactionRequest{History: history})
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	// Drop 1 of 3 turns: the whole tool-use turn (messages 0-3).
	if res.Cut != 4 {
		t.Fatalf("Cut = %d, want 4 (whole tool-use turn)", res.Cut)
	}
	if len(res.Replacement) != 0 {
		t.Fatalf("truncation must not produce a replacement, got %d messages", len(res.Replacement))
	}
	if res.Usage.InputTokens != 0 || res.Usage.OutputTokens != 0 {
		t.Fatalf("no-LLM strategy reported usage: %+v", res.Usage)
	}
	if err := validateHistory(history[res.Cut:]); err != nil {
		t.Fatalf("retained window violates invariants: %v", err)
	}
}

func TestSlidingWindow_NoOpWithinWindow(t *testing.T) {
	s := &SlidingWindowStrategy{KeepTurns: 3}
	res, err := s.Compact(context.Background(), CompactionRequest{History: toolTurnHistory()})
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	if res.Cut != 0 {
		t.Fatalf("expected no-op within window, got Cut=%d", res.Cut)
	}

	// Zero KeepTurns disables the strategy rather than dropping everything.
	disabled := &SlidingWindowStrategy{}
	res, err = disabled.Compact(context.Background(), CompactionRequest{History: toolTurnHistory()})
	if err != nil || res.Cut != 0 {
		t.Fatalf("expected disabled strategy to no-op, got Cut=%d err=%v", res.Cut, err)
	}
}

// S2.18 cache guidance: with AdvanceChunk set, the window's leading
// edge advances in chunks, not every turn.
func TestSlidingWindow_ChunkedAdvance(t *testing.T) {
	s := &SlidingWindowStrategy{KeepTurns: 2, AdvanceChunk: 3}

	// 4 turns: excess of 2 rounds down to 0 — no advance yet.
	history := toolTurnHistory()
	history = append(history, userText("u4"), assistantText("a4"))
	res, err := s.Compact(context.Background(), CompactionRequest{History: history})
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	if res.Cut != 0 {
		t.Fatalf("expected no advance below chunk size, got Cut=%d", res.Cut)
	}

	// 5 turns: excess of 3 hits the chunk — drop exactly 3 turns.
	history = append(history, userText("u5"), assistantText("a5"))
	res, err = s.Compact(context.Background(), CompactionRequest{History: history})
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	starts := turnStarts(history)
	if res.Cut != starts[3] {
		t.Fatalf("Cut = %d, want %d (three whole turns)", res.Cut, starts[3])
	}
}

func TestSlidingWindow_CommitmentModes(t *testing.T) {
	if (&SlidingWindowStrategy{KeepTurns: 2}).Committed() {
		t.Fatal("sliding window should default to transient application")
	}
	if !(&SlidingWindowStrategy{KeepTurns: 2, Commit: true}).Committed() {
		t.Fatal("Commit: true should make the strategy committed")
	}
}

// S6.36: the hybrid strategy invokes the Completer to produce a summary
// message and is committed; the summary plus the retained window
// satisfies the S2.15 invariants.
func TestHybridSummarization_SummaryAndWindow(t *testing.T) {
	history := toolTurnHistory()
	mock := &mockCompleter{response: "the user asked about times and tools"}
	s := &HybridSummarizationStrategy{KeepTurns: 1}

	if !s.Committed() {
		t.Fatal("hybrid summarization must always be committed")
	}

	res, err := s.Compact(context.Background(), CompactionRequest{
		History:   history,
		Completer: mock,
		Model:     "claude-sonnet-4-5",
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	// Retained fresh window is turn 3 (messages 6-7); the cut lands on
	// message 5, the assistant close of turn 2, so the user-role
	// summary alternates correctly.
	if res.Cut != 5 {
		t.Fatalf("Cut = %d, want 5", res.Cut)
	}
	if len(res.Replacement) != 1 {
		t.Fatalf("expected a single summary message, got %d", len(res.Replacement))
	}
	summary := res.Replacement[0]
	if summary.Role != anthropic.MessageParamRoleUser {
		t.Fatalf("summary role = %s, want user", summary.Role)
	}
	if got := summary.Content[0].OfText.Text; !strings.Contains(got, "the user asked about times and tools") {
		t.Fatalf("summary text missing completion: %q", got)
	}

	compacted := append(res.Replacement, history[res.Cut:]...)
	if err := validateHistory(compacted); err != nil {
		t.Fatalf("summary + retained window violates invariants: %v", err)
	}

	// The summary call flattened the prefix into a transcript: one
	// protocol-valid user message covering text and tool activity.
	if len(mock.capturedRequests) != 1 {
		t.Fatalf("expected exactly 1 Completer call, got %d", len(mock.capturedRequests))
	}
	sreq := mock.capturedRequests[0]
	if len(sreq.Messages) != 1 {
		t.Fatalf("summary request carried %d messages, want 1", len(sreq.Messages))
	}
	transcript := sreq.Messages[0].Content[0].OfText.Text
	for _, want := range []string{"u1", "get_time", "13:37", "a1 final", "u2"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("transcript missing %q:\n%s", want, transcript)
		}
	}
	if strings.Contains(transcript, "u3") {
		t.Fatalf("transcript leaked retained window content:\n%s", transcript)
	}

	// The summary call's usage is reported for S2.20 accumulation
	// (mock reports 10 input / 5 output).
	if res.Usage.InputTokens != 10 || res.Usage.OutputTokens != 5 {
		t.Fatalf("summary usage not propagated: %+v", res.Usage)
	}
}

func TestHybridSummarization_NoOpWithinWindow(t *testing.T) {
	s := &HybridSummarizationStrategy{KeepTurns: 3}
	mock := &mockCompleter{response: "unused"}
	res, err := s.Compact(context.Background(), CompactionRequest{History: toolTurnHistory(), Completer: mock})
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	if res.Cut != 0 {
		t.Fatalf("expected no-op within window, got Cut=%d", res.Cut)
	}
	if len(mock.capturedRequests) != 0 {
		t.Fatal("no-op must not invoke the Completer")
	}
}

// End-to-end (S6.36): an Agent with the hybrid strategy compacts via
// manual Compact; committed history is summary + retained window, it
// round-trips through NewAgentWithHistory, and the summarizer's usage
// lands in cumulative token usage (S2.20).
func TestHybridSummarization_EndToEndCommit(t *testing.T) {
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Text: "one"}, {Text: "two"}, {Text: "three"},
			{Text: "summary of one and two", InputTokens: 200, OutputTokens: 40},
		},
	}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	for _, msg := range []string{"first", "second", "third"} {
		if err := a.Run(context.Background(), msg, nil); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	var archived []anthropic.MessageParam
	a.SetCompaction(CompactionConfig{
		Strategy: &HybridSummarizationStrategy{KeepTurns: 1},
		Archive:  func(prefix []anthropic.MessageParam) { archived = prefix },
	})
	if err := a.Compact(context.Background()); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	// 6 messages -> summary + assistant("two") + user("third") + assistant("three").
	conv := a.Conversation()
	if len(conv) != 4 {
		t.Fatalf("expected 4 committed messages, got %d", len(conv))
	}
	if err := validateHistory(conv); err != nil {
		t.Fatalf("compacted history violates invariants: %v", err)
	}
	if len(archived) != 3 {
		t.Fatalf("expected 3 archived messages, got %d", len(archived))
	}
	if _, err := NewAgentWithHistory(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100}, conv); err != nil {
		t.Fatalf("resumption round-trip failed: %v", err)
	}

	// Cumulative usage includes the summarization call (S2.20): three
	// runs at the 10/5 defaults plus the 200/40 summary call.
	usage := a.Usage()
	if usage.Cumulative.InputTokens != 230 || usage.Cumulative.OutputTokens != 55 {
		t.Fatalf("cumulative usage missing summary call: %+v", usage.Cumulative)
	}
}
