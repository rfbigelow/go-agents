package agent

// EventType identifies the kind of streaming event.
type EventType int

const (
	// EventTextDelta is emitted when new text content arrives from the LLM.
	EventTextDelta EventType = iota

	// EventThinkingDelta is emitted when incremental Extended Thinking
	// text arrives from the LLM (S2.9). Only seen when thinking display
	// is "summarized".
	EventThinkingDelta

	// EventSignatureDelta is emitted once per thinking block, immediately
	// before the block ends, carrying the opaque thinking signature (S2.9).
	EventSignatureDelta

	// EventDone is emitted when the LLM response is complete.
	EventDone
)

// Event represents a streaming event from the LLM during a Run.
type Event struct {
	// Type identifies what kind of event this is.
	Type EventType

	// Text contains the text content for EventTextDelta events.
	Text string

	// Thinking contains the thinking text for EventThinkingDelta events.
	Thinking string

	// Signature contains the opaque thinking signature for
	// EventSignatureDelta events.
	Signature string
}
