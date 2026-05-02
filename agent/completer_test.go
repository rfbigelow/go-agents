package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// sseResponse builds an SSE response body for a simple text completion.
func sseResponse(text string) string {
	var b strings.Builder

	b.WriteString("event: message_start\n")
	b.WriteString(`data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`)
	b.WriteString("\n\n")

	b.WriteString("event: content_block_start\n")
	b.WriteString(`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	b.WriteString("\n\n")

	b.WriteString("event: content_block_delta\n")
	b.WriteString(fmt.Sprintf(`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`, text))
	b.WriteString("\n\n")

	b.WriteString("event: content_block_stop\n")
	b.WriteString(`data: {"type":"content_block_stop","index":0}`)
	b.WriteString("\n\n")

	b.WriteString("event: message_delta\n")
	b.WriteString(`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`)
	b.WriteString("\n\n")

	b.WriteString("event: message_stop\n")
	b.WriteString(`data: {"type":"message_stop"}`)
	b.WriteString("\n\n")

	return b.String()
}

// sseResponseWithThinking builds an SSE response body for a completion that
// includes a thinking block (with thinking_delta + signature_delta) followed
// by a text block.
func sseResponseWithThinking(thinking, signature, text string) string {
	var b strings.Builder

	b.WriteString("event: message_start\n")
	b.WriteString(`data: {"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-5-20250514","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`)
	b.WriteString("\n\n")

	b.WriteString("event: content_block_start\n")
	b.WriteString(`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`)
	b.WriteString("\n\n")

	b.WriteString("event: content_block_delta\n")
	b.WriteString(fmt.Sprintf(`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":%q}}`, thinking))
	b.WriteString("\n\n")

	b.WriteString("event: content_block_delta\n")
	b.WriteString(fmt.Sprintf(`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":%q}}`, signature))
	b.WriteString("\n\n")

	b.WriteString("event: content_block_stop\n")
	b.WriteString(`data: {"type":"content_block_stop","index":0}`)
	b.WriteString("\n\n")

	b.WriteString("event: content_block_start\n")
	b.WriteString(`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
	b.WriteString("\n\n")

	b.WriteString("event: content_block_delta\n")
	b.WriteString(fmt.Sprintf(`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":%q}}`, text))
	b.WriteString("\n\n")

	b.WriteString("event: content_block_stop\n")
	b.WriteString(`data: {"type":"content_block_stop","index":1}`)
	b.WriteString("\n\n")

	b.WriteString("event: message_delta\n")
	b.WriteString(`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":15}}`)
	b.WriteString("\n\n")

	b.WriteString("event: message_stop\n")
	b.WriteString(`data: {"type":"message_stop"}`)
	b.WriteString("\n\n")

	return b.String()
}

func newTestServer(handler http.HandlerFunc) (*httptest.Server, anthropic.Client) {
	server := httptest.NewServer(handler)
	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	return server, client
}

func TestAnthropicCompleter_SimpleCompletion(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseResponse("Hello, world!"))
	})
	defer server.Close()

	completer := NewAnthropicCompleter(client)
	stream, err := completer.Complete(context.Background(), CompletionRequest{
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 100,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	defer stream.Close()

	var texts []string
	for stream.Next() {
		e := stream.Event()
		if e.Type == EventTextDelta {
			texts = append(texts, e.Text)
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	if len(texts) != 1 || texts[0] != "Hello, world!" {
		t.Fatalf("expected [\"Hello, world!\"], got %v", texts)
	}

	msg := stream.Message()
	if msg.StopReason != anthropic.StopReasonEndTurn {
		t.Fatalf("expected stop_reason end_turn, got %v", msg.StopReason)
	}
}

func TestAnthropicCompleter_ThinkingAndSignatureDeltasSurface(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseResponseWithThinking("let me think...", "sig_abc", "the answer is 42"))
	})
	defer server.Close()

	completer := NewAnthropicCompleter(client)
	stream, err := completer.Complete(context.Background(), CompletionRequest{
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 100,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	defer stream.Close()

	var observed []Event
	for stream.Next() {
		observed = append(observed, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}

	if len(observed) != 3 {
		t.Fatalf("expected 3 events (thinking_delta, signature_delta, text_delta), got %d: %+v", len(observed), observed)
	}
	if observed[0].Type != EventThinkingDelta || observed[0].Thinking != "let me think..." {
		t.Errorf("event[0] = %+v, want EventThinkingDelta with thinking text", observed[0])
	}
	if observed[1].Type != EventSignatureDelta || observed[1].Signature != "sig_abc" {
		t.Errorf("event[1] = %+v, want EventSignatureDelta with signature", observed[1])
	}
	if observed[2].Type != EventTextDelta || observed[2].Text != "the answer is 42" {
		t.Errorf("event[2] = %+v, want EventTextDelta with text", observed[2])
	}
}

func ptrInt64(v int64) *int64    { return &v }
func ptrString(v string) *string { return &v }

func TestBuildThinkingParam_EnabledDefaultDisplay(t *testing.T) {
	got := buildThinkingParam(&ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64(2048)})
	if got.OfEnabled == nil {
		t.Fatalf("expected OfEnabled, got %+v", got)
	}
	if got.OfEnabled.BudgetTokens != 2048 {
		t.Errorf("BudgetTokens = %d, want 2048", got.OfEnabled.BudgetTokens)
	}
	if got.OfEnabled.Display != anthropic.ThinkingConfigEnabledDisplayOmitted {
		t.Errorf("Display = %q, want %q", got.OfEnabled.Display, anthropic.ThinkingConfigEnabledDisplayOmitted)
	}
}

func TestBuildThinkingParam_EnabledExplicitSummarized(t *testing.T) {
	got := buildThinkingParam(&ThinkingConfig{Type: "enabled", BudgetTokens: ptrInt64(1024), Display: ptrString("summarized")})
	if got.OfEnabled == nil {
		t.Fatalf("expected OfEnabled, got %+v", got)
	}
	if got.OfEnabled.Display != anthropic.ThinkingConfigEnabledDisplaySummarized {
		t.Errorf("Display = %q, want %q", got.OfEnabled.Display, anthropic.ThinkingConfigEnabledDisplaySummarized)
	}
}

func TestBuildThinkingParam_AdaptiveDefaultDisplay(t *testing.T) {
	got := buildThinkingParam(&ThinkingConfig{Type: "adaptive"})
	if got.OfAdaptive == nil {
		t.Fatalf("expected OfAdaptive, got %+v", got)
	}
	if got.OfAdaptive.Display != anthropic.ThinkingConfigAdaptiveDisplayOmitted {
		t.Errorf("Display = %q, want %q", got.OfAdaptive.Display, anthropic.ThinkingConfigAdaptiveDisplayOmitted)
	}
}

func TestBuildThinkingParam_AdaptiveExplicitSummarized(t *testing.T) {
	got := buildThinkingParam(&ThinkingConfig{Type: "adaptive", Display: ptrString("summarized")})
	if got.OfAdaptive == nil {
		t.Fatalf("expected OfAdaptive, got %+v", got)
	}
	if got.OfAdaptive.Display != anthropic.ThinkingConfigAdaptiveDisplaySummarized {
		t.Errorf("Display = %q, want %q", got.OfAdaptive.Display, anthropic.ThinkingConfigAdaptiveDisplaySummarized)
	}
}

func TestBuildThinkingParam_DisabledNoDisplay(t *testing.T) {
	got := buildThinkingParam(&ThinkingConfig{Type: "disabled"})
	if got.OfDisabled == nil {
		t.Fatalf("expected OfDisabled, got %+v", got)
	}
	if got.OfEnabled != nil || got.OfAdaptive != nil {
		t.Errorf("expected only OfDisabled set, got %+v", got)
	}
}

func TestBuildThinkingParam_UnknownTypeYieldsEmptyUnion(t *testing.T) {
	got := buildThinkingParam(&ThinkingConfig{Type: "experimental_x"})
	if got.OfEnabled != nil || got.OfAdaptive != nil || got.OfDisabled != nil {
		t.Errorf("expected empty union for unknown type, got %+v", got)
	}
}

func TestAnthropicCompleter_APIError(t *testing.T) {
	server, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`)
	})
	defer server.Close()

	completer := NewAnthropicCompleter(client)
	stream, err := completer.Complete(context.Background(), CompletionRequest{
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 100,
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
	})

	// The SDK may return the error immediately or after the first Next() call.
	// Either way, we should get an error.
	if err != nil {
		// Error returned from Complete directly — expected for non-streaming errors
		return
	}
	defer stream.Close()

	for stream.Next() {
		// consume
	}
	if stream.Err() == nil {
		t.Fatal("expected error from API, got nil")
	}
}
