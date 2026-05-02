package agent

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// Completer abstracts LLM communication for the Agent.
type Completer interface {
	Complete(ctx context.Context, req CompletionRequest) (*EventStream, error)
}

// CompletionRequest contains all parameters for a single LLM completion.
type CompletionRequest struct {
	Messages    []anthropic.MessageParam
	Model       anthropic.Model
	MaxTokens   int64
	System      []anthropic.TextBlockParam
	Tools       []anthropic.ToolUnionParam
	Temperature *float64
	Thinking    *ThinkingConfig
	Effort      *string
}

// EventStream provides typed streaming events and accumulates the
// complete Message. It wraps an eventSource which can be backed by
// the SDK stream (production) or a mock (testing).
type EventStream struct {
	source  eventSource
	message anthropic.Message
	event   Event
}

// eventSource abstracts the underlying event provider.
type eventSource interface {
	Next() bool
	Current() anthropic.MessageStreamEventUnion
	Err() error
	Close() error
	// accumulates returns true if events should be passed to Message.Accumulate.
	// Test sources return false because their events lack the JSON backing
	// that Accumulate requires.
	accumulates() bool
}

// sdkSource wraps the SDK's Stream to implement eventSource.
type sdkSource struct {
	stream *ssestream.Stream[anthropic.MessageStreamEventUnion]
}

func (s *sdkSource) Next() bool                                    { return s.stream.Next() }
func (s *sdkSource) Current() anthropic.MessageStreamEventUnion    { return s.stream.Current() }
func (s *sdkSource) Err() error                                    { return s.stream.Err() }
func (s *sdkSource) Close() error                                  { return s.stream.Close() }
func (s *sdkSource) accumulates() bool                             { return true }

// newEventStream creates an EventStream from an SDK stream.
func newEventStream(stream *ssestream.Stream[anthropic.MessageStreamEventUnion]) *EventStream {
	return &EventStream{source: &sdkSource{stream: stream}}
}

// NewTestEventStream creates an EventStream for testing that yields the
// given text deltas and returns the provided message when fully consumed.
func NewTestEventStream(texts []string, msg anthropic.Message) *EventStream {
	events := make([]testEvent, 0, len(texts))
	for _, t := range texts {
		events = append(events, testEvent{Kind: "text", Value: t})
	}
	return &EventStream{source: &testSource{events: events, idx: -1}, message: msg}
}

// NewTestEventStreamFromEvents creates an EventStream for testing that
// yields a heterogeneous sequence of text/thinking/signature deltas and
// returns the provided message when fully consumed.
func NewTestEventStreamFromEvents(events []testEvent, msg anthropic.Message) *EventStream {
	return &EventStream{source: &testSource{events: events, idx: -1}, message: msg}
}

// NewTestEventStreamError creates an EventStream for testing that returns
// the given error.
func NewTestEventStreamError(err error) *EventStream {
	return &EventStream{source: &testSource{err: err}}
}

// Next advances to the next event. Returns false when the stream is
// exhausted or an error occurs. Check Err() after Next returns false.
func (es *EventStream) Next() bool {
	for es.source.Next() {
		raw := es.source.Current()
		if es.source.accumulates() {
			es.message.Accumulate(raw)
		}

		if raw.Type == "content_block_delta" {
			switch raw.Delta.Type {
			case "text_delta":
				es.event = Event{Type: EventTextDelta, Text: raw.Delta.Text}
				return true
			case "thinking_delta":
				es.event = Event{Type: EventThinkingDelta, Thinking: raw.Delta.Thinking}
				return true
			case "signature_delta":
				es.event = Event{Type: EventSignatureDelta, Signature: raw.Delta.Signature}
				return true
			}
		}
	}
	return false
}

// Event returns the current streaming event.
func (es *EventStream) Event() Event {
	return es.event
}

// Err returns the error that caused the stream to end, if any.
func (es *EventStream) Err() error {
	return es.source.Err()
}

// Message returns the accumulated complete message.
// Only valid after the stream is fully consumed (Next returns false).
func (es *EventStream) Message() anthropic.Message {
	return es.message
}

// Close closes the underlying stream.
func (es *EventStream) Close() error {
	return es.source.Close()
}

// testEvent describes a single content_block_delta to surface from a
// testSource. Kind is one of "text", "thinking", "signature".
type testEvent struct {
	Kind  string
	Value string
}

// testSource yields pre-built Event values directly, bypassing the
// SDK's Accumulate path. The EventStream's message field is set at
// construction time by NewTestEventStream.
type testSource struct {
	events []testEvent
	idx    int
	err    error
}

func (s *testSource) Next() bool {
	if s.err != nil {
		return false
	}
	s.idx++
	return s.idx < len(s.events)
}

func (s *testSource) Current() anthropic.MessageStreamEventUnion {
	e := s.events[s.idx]
	delta := anthropic.MessageStreamEventUnionDelta{}
	switch e.Kind {
	case "text":
		delta.Type = "text_delta"
		delta.Text = e.Value
	case "thinking":
		delta.Type = "thinking_delta"
		delta.Thinking = e.Value
	case "signature":
		delta.Type = "signature_delta"
		delta.Signature = e.Value
	}
	return anthropic.MessageStreamEventUnion{
		Type:  "content_block_delta",
		Delta: delta,
	}
}

func (s *testSource) Err() error        { return s.err }
func (s *testSource) Close() error      { return nil }
func (s *testSource) accumulates() bool { return false }

// buildThinkingParam translates a library ThinkingConfig into the SDK's
// typed thinking union (S2.9). Unknown Type values yield an empty union
// (boundary-level passthrough only — the SDK has no slot for them).
func buildThinkingParam(t *ThinkingConfig) anthropic.ThinkingConfigParamUnion {
	switch t.Type {
	case "enabled":
		p := &anthropic.ThinkingConfigEnabledParam{}
		if t.BudgetTokens != nil {
			p.BudgetTokens = *t.BudgetTokens
		}
		p.Display = resolveEnabledDisplay(t.Display)
		return anthropic.ThinkingConfigParamUnion{OfEnabled: p}
	case "adaptive":
		p := &anthropic.ThinkingConfigAdaptiveParam{}
		p.Display = resolveAdaptiveDisplay(t.Display)
		return anthropic.ThinkingConfigParamUnion{OfAdaptive: p}
	case "disabled":
		return anthropic.ThinkingConfigParamUnion{
			OfDisabled: &anthropic.ThinkingConfigDisabledParam{},
		}
	}
	return anthropic.ThinkingConfigParamUnion{}
}

// resolveEnabledDisplay applies the library-side default of "omitted" when
// no explicit display was set on a Type=enabled thinking config (S2.9).
func resolveEnabledDisplay(d *string) anthropic.ThinkingConfigEnabledDisplay {
	if d == nil {
		return anthropic.ThinkingConfigEnabledDisplayOmitted
	}
	return anthropic.ThinkingConfigEnabledDisplay(*d)
}

// resolveAdaptiveDisplay applies the library-side default of "omitted" when
// no explicit display was set on a Type=adaptive thinking config (S2.9).
func resolveAdaptiveDisplay(d *string) anthropic.ThinkingConfigAdaptiveDisplay {
	if d == nil {
		return anthropic.ThinkingConfigAdaptiveDisplayOmitted
	}
	return anthropic.ThinkingConfigAdaptiveDisplay(*d)
}

// AnthropicCompleter is the library-provided Completer implementation.
// It acts as a stateless Adapter for an Anthropic client.
type AnthropicCompleter struct {
	client anthropic.Client
}

// NewAnthropicCompleter creates a Completer that wraps the given Anthropic client.
func NewAnthropicCompleter(client anthropic.Client) *AnthropicCompleter {
	return &AnthropicCompleter{client: client}
}

// Complete sends a completion request to the Anthropic API and returns
// a streaming response.
func (c *AnthropicCompleter) Complete(ctx context.Context, req CompletionRequest) (*EventStream, error) {
	params := anthropic.MessageNewParams{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Messages:  req.Messages,
	}

	if len(req.System) > 0 {
		params.System = req.System
	}

	if len(req.Tools) > 0 {
		params.Tools = req.Tools
	}

	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}

	if req.Thinking != nil {
		params.Thinking = buildThinkingParam(req.Thinking)
	}

	if req.Effort != nil {
		params.OutputConfig = anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(*req.Effort),
		}
	}

	stream := c.client.Messages.NewStreaming(ctx, params)
	if err := stream.Err(); err != nil {
		stream.Close()
		return nil, fmt.Errorf("starting stream: %w", err)
	}

	return newEventStream(stream), nil
}
