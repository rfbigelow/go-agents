package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// testStrategy is a scriptable CompactionStrategy for engine and
// trigger tests. It cuts the first Cut messages of the supplied
// history (clamped to its length) and replaces them with Replacement.
type testStrategy struct {
	committed   bool
	cut         int
	replacement []anthropic.MessageParam
	usage       anthropic.Usage
	err         error
	calls       int
	lastHistory []anthropic.MessageParam
}

func (s *testStrategy) Compact(_ context.Context, req CompactionRequest) (CompactionResult, error) {
	s.calls++
	s.lastHistory = req.History
	if s.err != nil {
		return CompactionResult{}, s.err
	}
	cut := s.cut
	if cut > len(req.History) {
		cut = len(req.History)
	}
	return CompactionResult{Cut: cut, Replacement: s.replacement, Usage: s.usage}, nil
}

func (s *testStrategy) Committed() bool { return s.committed }

// overflowError builds the Anthropic API's context-window overflow
// error shape: HTTP 400 invalid_request_error whose message reports
// the token excess.
func overflowError() error {
	var e anthropic.Error
	body := `{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 210000 tokens > 200000 maximum"}}`
	if err := e.UnmarshalJSON([]byte(body)); err != nil {
		panic(fmt.Sprintf("overflowError unmarshal: %v", err))
	}
	e.StatusCode = 400
	// The SDK's Error() formats from the originating request/response;
	// populate the minimum so err.Error() is callable in logs/spans.
	e.Request, _ = http.NewRequest("POST", "https://api.anthropic.com/v1/messages", nil)
	e.Response = &http.Response{StatusCode: 400}
	return &e
}

// overflowNCompleter wraps mockCompleter, returning a context-window
// overflow error for the first failCalls Complete calls and delegating
// to the inner script afterwards.
type overflowNCompleter struct {
	inner     mockCompleter
	failCalls int
	calls     int
}

func (c *overflowNCompleter) Complete(ctx context.Context, req CompletionRequest) (*EventStream, error) {
	c.calls++
	if c.calls <= c.failCalls {
		c.inner.capturedRequests = append(c.inner.capturedRequests, req)
		return nil, overflowError()
	}
	return c.inner.Complete(ctx, req)
}

func TestIsContextOverflow(t *testing.T) {
	if !isContextOverflow(fmt.Errorf("completing: %w", overflowError())) {
		t.Fatal("expected wrapped overflow error to be detected")
	}
	if isContextOverflow(fmt.Errorf("some other error")) {
		t.Fatal("expected non-API error not to be detected")
	}
	var e anthropic.Error
	if err := e.UnmarshalJSON([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens is required"}}`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	e.StatusCode = 400
	if isContextOverflow(&e) {
		t.Fatal("expected unrelated 400 not to be detected as overflow")
	}
}

// S6.36: with no strategy configured, history is never compacted —
// messages after a multi-turn run equals the full unaltered history,
// and manual Compact is a no-op.
func TestCompaction_DisabledByDefault(t *testing.T) {
	registry := NewToolRegistry()
	mustRegister(t, registry, Tool{
		Name:    "noop",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) { return "ok", nil },
	})
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{ToolCalls: []scriptedToolCall{{ID: "t1", Name: "noop", Input: json.RawMessage(`{}`)}}},
			{Text: "done"},
		},
	}
	a := NewAgent(mock, registry, Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	if err := a.Run(context.Background(), "go", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if err := a.Compact(context.Background()); err != nil {
		t.Fatalf("Compact without strategy should be a no-op, got %v", err)
	}
	if got := len(a.Conversation()); got != 4 {
		t.Fatalf("expected full unaltered history of 4 messages, got %d", got)
	}
}

// S6.36: with a committing strategy configured and its trigger met, the
// committed history is replaced by the compacted form; messages on the
// resulting Agent satisfies the five S2.15 invariants and round-trips
// through NewAgentWithHistory. The replaced prefix is delivered to the
// archival callback in the SDK-native representation before being
// discarded.
func TestCompaction_ManualCommitAndArchive(t *testing.T) {
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Text: "one"}, {Text: "two"}, {Text: "three"},
		},
	}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	for _, msg := range []string{"first", "second", "third"} {
		if err := a.Run(context.Background(), msg, nil); err != nil {
			t.Fatalf("Run(%q) failed: %v", msg, err)
		}
	}
	full := a.Conversation() // 6 messages: 3 user/assistant pairs

	var archived []anthropic.MessageParam
	strategy := &testStrategy{committed: true, cut: 4}
	a.SetCompaction(CompactionConfig{
		Strategy: strategy,
		Archive:  func(prefix []anthropic.MessageParam) { archived = prefix },
	})

	if err := a.Compact(context.Background()); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	compacted := a.Conversation()
	if len(compacted) != 2 {
		t.Fatalf("expected 2 messages after compaction, got %d", len(compacted))
	}
	if err := validateHistory(compacted); err != nil {
		t.Fatalf("compacted history violates resumption invariants: %v", err)
	}
	if len(archived) != 4 {
		t.Fatalf("expected 4 archived messages, got %d", len(archived))
	}
	// Archived prefix is the replaced messages in SDK-native form.
	for i := range archived {
		want, _ := json.Marshal(full[i])
		got, _ := json.Marshal(archived[i])
		if string(want) != string(got) {
			t.Fatalf("archived[%d] != original message: %s vs %s", i, got, want)
		}
	}

	// Round-trip: the compacted history constructs a new Agent (S2.15).
	if _, err := NewAgentWithHistory(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100}, compacted); err != nil {
		t.Fatalf("compacted history failed resumption round-trip: %v", err)
	}
}

// S6.36: with no archival callback registered, compaction still
// succeeds and the prefix is dropped.
func TestCompaction_NoArchiveCallback(t *testing.T) {
	mock := &mockCompleter{responses: []scriptedResponse{{Text: "one"}, {Text: "two"}}}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	for _, msg := range []string{"first", "second"} {
		if err := a.Run(context.Background(), msg, nil); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	a.SetCompaction(CompactionConfig{Strategy: &testStrategy{committed: true, cut: 2}})
	if err := a.Compact(context.Background()); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	if got := len(a.Conversation()); got != 2 {
		t.Fatalf("expected 2 messages after compaction, got %d", got)
	}
}

// S6.36 (invariant preservation): a strategy proposing a cut that
// violates the protocol invariants is rejected — the error surfaces
// and committed history is left untouched.
func TestCompaction_InvalidStrategyOutputRejected(t *testing.T) {
	mock := &mockCompleter{responses: []scriptedResponse{{Text: "one"}, {Text: "two"}}}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	for _, msg := range []string{"first", "second"} {
		if err := a.Run(context.Background(), msg, nil); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Cutting 1 message strands the leading assistant message —
	// alternation (rule 2) breaks.
	a.SetCompaction(CompactionConfig{Strategy: &testStrategy{committed: true, cut: 1}})
	err := a.Compact(context.Background())
	if err == nil {
		t.Fatal("expected Compact to reject invariant-violating cut")
	}
	var hve *HistoryValidationError
	if !errors.As(err, &hve) {
		t.Fatalf("expected HistoryValidationError in chain, got %v", err)
	}
	if got := len(a.Conversation()); got != 4 {
		t.Fatalf("history mutated despite rejected compaction: %d messages", got)
	}
}

// S6.36 (commitment semantics): manual Compact always commits, even
// for a strategy that reports transient capability — the committed
// history is mutated and the archival callback fires.
func TestCompaction_ManualCommitsTransientStrategy(t *testing.T) {
	mock := &mockCompleter{responses: []scriptedResponse{{Text: "one"}, {Text: "two"}}}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	for _, msg := range []string{"first", "second"} {
		if err := a.Run(context.Background(), msg, nil); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	var archived []anthropic.MessageParam
	a.SetCompaction(CompactionConfig{
		Strategy: &testStrategy{committed: false, cut: 2},
		Archive:  func(prefix []anthropic.MessageParam) { archived = prefix },
	})
	if err := a.Compact(context.Background()); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
	if got := len(a.Conversation()); got != 2 {
		t.Fatalf("expected manual Compact to commit (2 messages), got %d", got)
	}
	if len(archived) != 2 {
		t.Fatalf("expected archival callback to fire with 2 messages, got %d", len(archived))
	}
}

// S6.37: proactive trigger — with a token threshold configured,
// compaction is applied before the LLM call once usage crosses the
// threshold, and not before.
func TestCompaction_ProactiveThreshold(t *testing.T) {
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Text: "one", InputTokens: 50},                            // below threshold
			{Text: "two", InputTokens: 120, CacheReadInputTokens: 30}, // crosses it
			{Text: "three"},
		},
	}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	strategy := &testStrategy{committed: true, cut: 2}
	a.SetCompaction(CompactionConfig{Strategy: strategy, TokenThreshold: 100})

	if err := a.Run(context.Background(), "first", nil); err != nil {
		t.Fatalf("first Run failed: %v", err)
	}
	if strategy.calls != 0 {
		t.Fatalf("strategy fired below threshold: %d calls", strategy.calls)
	}
	if err := a.Run(context.Background(), "second", nil); err != nil {
		t.Fatalf("second Run failed: %v", err)
	}
	// 50 < 100: second run's call goes out uncompacted; it reports 150
	// input-side tokens (120 + 30 cache read), crossing the threshold.
	if strategy.calls != 0 {
		t.Fatalf("strategy fired before usage crossed threshold: %d calls", strategy.calls)
	}
	if err := a.Run(context.Background(), "third", nil); err != nil {
		t.Fatalf("third Run failed: %v", err)
	}
	if strategy.calls != 1 {
		t.Fatalf("expected exactly 1 strategy call after crossing threshold, got %d", strategy.calls)
	}
	// The strategy saw only the pre-run committed prefix (4 messages),
	// not the third run's pending user message.
	if got := len(strategy.lastHistory); got != 4 {
		t.Fatalf("strategy saw %d messages, want 4 (pre-run committed prefix)", got)
	}
	// The compacted view went out on the wire: third request has
	// 4-2+1 = 3 messages (2 retained + pending user).
	third := mock.capturedRequests[2]
	if got := len(third.Messages); got != 3 {
		t.Fatalf("third request carried %d messages, want 3 (compacted)", got)
	}
	if err := validateHistory(a.Conversation()); err != nil {
		t.Fatalf("post-run history violates invariants: %v", err)
	}
}

// S6.37 (transient application): a transient strategy shapes the
// outgoing request without mutating committed state — messages still
// returns the full history.
func TestCompaction_ProactiveTransient(t *testing.T) {
	mock := &mockCompleter{
		responses: []scriptedResponse{
			{Text: "one", InputTokens: 150},
			{Text: "two"},
		},
	}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	strategy := &testStrategy{committed: false, cut: 2}
	var archived bool
	a.SetCompaction(CompactionConfig{
		Strategy: strategy,
		// Relative threshold form: 0.5 * 200 = 100 tokens.
		ThresholdFraction: 0.5,
		ContextWindow:     200,
		Archive:           func([]anthropic.MessageParam) { archived = true },
	})

	if err := a.Run(context.Background(), "first", nil); err != nil {
		t.Fatalf("first Run failed: %v", err)
	}
	if err := a.Run(context.Background(), "second", nil); err != nil {
		t.Fatalf("second Run failed: %v", err)
	}
	if strategy.calls != 1 {
		t.Fatalf("expected 1 strategy call, got %d", strategy.calls)
	}
	// Wire view compacted: 2 committed - 2 cut + pending user = 1.
	second := mock.capturedRequests[1]
	if got := len(second.Messages); got != 1 {
		t.Fatalf("second request carried %d messages, want 1 (transient view)", got)
	}
	// Committed state untouched: 4 messages, nothing archived.
	if got := len(a.Conversation()); got != 4 {
		t.Fatalf("transient compaction mutated committed state: %d messages", got)
	}
	if archived {
		t.Fatal("transient compaction must not archive — nothing is discarded")
	}
}

// S6.37: reactive trigger — when a call overflows and a strategy is
// configured, the library compacts and retries the call once, and the
// retried request reflects the compacted history.
func TestCompaction_ReactiveRetry(t *testing.T) {
	completer := &overflowNCompleter{
		inner:     mockCompleter{responses: []scriptedResponse{{Text: "recovered"}}},
		failCalls: 1,
	}
	a := NewAgent(completer, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	// Seed two turns of committed history via resumption-valid construction.
	seed := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("old question")),
		assistantText("old answer"),
	}
	a.conversation = ConversationState{messages: seed}

	strategy := &testStrategy{committed: true, cut: 2}
	a.SetCompaction(CompactionConfig{Strategy: strategy})

	if err := a.Run(context.Background(), "new question", nil); err != nil {
		t.Fatalf("Run failed despite reactive compaction: %v", err)
	}
	if strategy.calls != 1 {
		t.Fatalf("expected 1 strategy call, got %d", strategy.calls)
	}
	reqs := completer.inner.capturedRequests
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (original + retry), got %d", len(reqs))
	}
	if got := len(reqs[0].Messages); got != 3 {
		t.Fatalf("original request carried %d messages, want 3", got)
	}
	// Retry reflects the compacted history: 3 - 2 = 1 message.
	if got := len(reqs[1].Messages); got != 1 {
		t.Fatalf("retried request carried %d messages, want 1 (compacted)", got)
	}
	// Committed history: compacted prefix (0) + user + assistant.
	conv := a.Conversation()
	if len(conv) != 2 {
		t.Fatalf("expected 2 committed messages after reactive compaction, got %d", len(conv))
	}
}

// S6.37: if the request still overflows after one compaction, the
// error is returned to the consumer and the offending message is not
// appended.
func TestCompaction_ReactiveStillOverflows(t *testing.T) {
	completer := &overflowNCompleter{
		inner:     mockCompleter{response: "unreachable"},
		failCalls: 99, // every call overflows
	}
	a := NewAgent(completer, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	seed := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("old question")),
		assistantText("old answer"),
	}
	a.conversation = ConversationState{messages: seed}
	strategy := &testStrategy{committed: true, cut: 2}
	a.SetCompaction(CompactionConfig{Strategy: strategy})

	err := a.Run(context.Background(), "new question", nil)
	if err == nil {
		t.Fatal("expected overflow error to surface")
	}
	if !isContextOverflow(err) {
		t.Fatalf("expected context-overflow error, got %v", err)
	}
	if completer.calls != 2 {
		t.Fatalf("expected exactly 2 calls (original + one retry), got %d", completer.calls)
	}
	// The offending user message is not appended (turn-0 rollback);
	// the committed compaction stands.
	if got := len(a.Conversation()); got != 0 {
		t.Fatalf("expected 0 committed messages (prefix compacted away, user rolled back), got %d", got)
	}
}

// S6.37: with no strategy configured, an overflow error is returned to
// the consumer without any retry.
func TestCompaction_ReactiveNoStrategy(t *testing.T) {
	completer := &overflowNCompleter{inner: mockCompleter{}, failCalls: 99}
	a := NewAgent(completer, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})

	err := a.Run(context.Background(), "question", nil)
	if err == nil {
		t.Fatal("expected overflow error to surface")
	}
	if completer.calls != 1 {
		t.Fatalf("expected exactly 1 call (no retry without strategy), got %d", completer.calls)
	}
	if got := len(a.Conversation()); got != 0 {
		t.Fatalf("expected offending message not appended, got %d messages", got)
	}
}

// S2.20: a summarizing strategy's own usage counts toward cumulative
// usage — and toward the last-run component only when triggered
// mid-run, not by manual Compact.
func TestCompaction_StrategyUsageCounted(t *testing.T) {
	mock := &mockCompleter{responses: []scriptedResponse{{Text: "one", InputTokens: 40, OutputTokens: 10}}}
	a := NewAgent(mock, NewToolRegistry(), Config{Model: "claude-sonnet-4-5", MaxTokens: 100})
	if err := a.Run(context.Background(), "first", nil); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	a.SetCompaction(CompactionConfig{Strategy: &testStrategy{
		committed: true,
		cut:       2,
		usage:     anthropic.Usage{InputTokens: 500, OutputTokens: 50},
	}})
	if err := a.Compact(context.Background()); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	usage := a.Usage()
	wantCumulative := UsageTotals{InputTokens: 540, OutputTokens: 60}
	if usage.Cumulative != wantCumulative {
		t.Fatalf("Cumulative = %+v, want %+v (run + summarization)", usage.Cumulative, wantCumulative)
	}
	wantLast := UsageTotals{InputTokens: 40, OutputTokens: 10}
	if usage.LastRun != wantLast {
		t.Fatalf("LastRun = %+v, want %+v (manual Compact is not a run)", usage.LastRun, wantLast)
	}
}
