package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// S6.38: after a run, the usage query returns the per-run and cumulative
// input, output, and cache (creation/read) token counts matching the
// mocked responses. A tool-use run makes two LLM calls; the run's usage
// is their sum.
func TestAgent_Usage_PerRunMatchesMockedResponses(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name: "get_time",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "13:37", nil
		},
	})

	mock := &mockCompleter{
		responses: []scriptedResponse{
			{
				ToolCalls:                []scriptedToolCall{{ID: "toolu_1", Name: "get_time", Input: json.RawMessage(`{}`)}},
				InputTokens:              100,
				OutputTokens:             20,
				CacheCreationInputTokens: 80,
			},
			{
				Text:                 "It is 13:37.",
				InputTokens:          30,
				OutputTokens:         7,
				CacheReadInputTokens: 150,
			},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	if got := a.Usage(); got != (TokenUsage{}) {
		t.Fatalf("expected zero usage on a new Agent, got %+v", got)
	}

	if err := a.Run(context.Background(), "what time?", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	want := UsageTotals{
		InputTokens:              130,
		OutputTokens:             27,
		CacheCreationInputTokens: 80,
		CacheReadInputTokens:     150,
	}
	usage := a.Usage()
	if usage.LastRun != want {
		t.Fatalf("LastRun = %+v, want %+v", usage.LastRun, want)
	}
	if usage.Cumulative != want {
		t.Fatalf("Cumulative = %+v, want %+v", usage.Cumulative, want)
	}
}

// S6.38: cumulative usage accumulates across successive runs in the same
// conversation, while each run replaces the last-run component.
func TestAgent_Usage_CumulativeAcrossRuns(t *testing.T) {
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Text: "one", InputTokens: 40, OutputTokens: 10},
			{Text: "two", InputTokens: 60, OutputTokens: 15, CacheReadInputTokens: 35},
		},
	}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	if err := a.Run(context.Background(), "first", nil); err != nil {
		t.Fatalf("first Run failed: %v", err)
	}
	if err := a.Run(context.Background(), "second", nil); err != nil {
		t.Fatalf("second Run failed: %v", err)
	}

	usage := a.Usage()
	wantLast := UsageTotals{InputTokens: 60, OutputTokens: 15, CacheReadInputTokens: 35}
	if usage.LastRun != wantLast {
		t.Fatalf("LastRun = %+v, want %+v (second run only)", usage.LastRun, wantLast)
	}
	wantCumulative := UsageTotals{InputTokens: 100, OutputTokens: 25, CacheReadInputTokens: 35}
	if usage.Cumulative != wantCumulative {
		t.Fatalf("Cumulative = %+v, want %+v", usage.Cumulative, wantCumulative)
	}
}

// S6.38: reporting is read-only — reading usage does not alter
// conversation state or behavior, and an Agent resumed from history
// starts with zero usage (per the Agent ADT command-query table).
func TestAgent_Usage_ReadOnlyAndZeroOnResume(t *testing.T) {
	mock := &mockCompleter{response: "hello"}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	if err := a.Run(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	before := a.Conversation()
	first := a.Usage()
	second := a.Usage()
	if first != second {
		t.Fatalf("Usage() not stable across reads: %+v then %+v", first, second)
	}
	after := a.Conversation()
	if len(before) != len(after) {
		t.Fatalf("Usage() altered conversation length: %d -> %d", len(before), len(after))
	}

	resumed, err := NewAgentWithHistory(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100}, a.Conversation())
	if err != nil {
		t.Fatalf("NewAgentWithHistory failed: %v", err)
	}
	if got := resumed.Usage(); got != (TokenUsage{}) {
		t.Fatalf("expected zero usage on resumed Agent, got %+v", got)
	}
}
