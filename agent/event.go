package agent

// EventType identifies the kind of streaming event.
type EventType int

const (
	// EventTextDelta is emitted when new text content arrives from the LLM.
	EventTextDelta EventType = iota

	// EventDone is emitted when the LLM response is complete.
	EventDone
)

// Event represents a streaming event from the LLM during a Run.
type Event struct {
	// Type identifies what kind of event this is.
	Type EventType

	// Text contains the text content for EventTextDelta events.
	Text string
}
