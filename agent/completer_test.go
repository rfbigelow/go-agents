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
