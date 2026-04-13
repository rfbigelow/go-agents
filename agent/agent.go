package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"go.opentelemetry.io/otel/attribute"
)

// Agent manages the agentic loop, coordinating LLM communication via the
// Completer, tool dispatch via the ToolRegistry, and conversation history
// via ConversationState.
type Agent struct {
	completer    Completer
	registry     *ToolRegistry
	config       Config
	conversation ConversationState
	log          *slog.Logger
}

// NewAgent creates an Agent with the given Completer, ToolRegistry, and Config.
func NewAgent(completer Completer, registry *ToolRegistry, config Config) *Agent {
	return &Agent{
		completer: completer,
		registry:  registry,
		config:    config,
		log:       config.logger(),
	}
}

// Conversation returns the current conversation state.
func (a *Agent) Conversation() []anthropic.MessageParam {
	return a.conversation.Messages()
}

// EventHandler is a callback invoked for each streaming event during a Run.
type EventHandler func(Event)

// Run executes the agentic loop for a single user message. The handler
// callback is invoked for each streaming event. Run blocks until the
// agentic loop completes or an error occurs.
func (a *Agent) Run(ctx context.Context, message string, handler EventHandler) error {
	ctx, span := startSpan(ctx, "agent.run",
		attribute.String("agent.model", string(a.config.Model)),
	)
	var runErr error
	defer func() { endSpan(span, runErr) }()

	a.log.InfoContext(ctx, "run started",
		logArgs(ctx, "model", string(a.config.Model))...,
	)

	// Append user message to conversation
	a.conversation.Append(anthropic.NewUserMessage(anthropic.NewTextBlock(message)))

	// Build the completion request
	req := a.buildRequest()

	// Call the Completer
	response, err := a.complete(ctx, req, handler)
	if err != nil {
		// Roll back the user message on error
		a.conversation.Rollback(1)
		runErr = err
		a.log.ErrorContext(ctx, "run failed",
			logArgs(ctx, "error", err.Error())...,
		)
		return fmt.Errorf("agent run: %w", err)
	}

	// Append assistant response to conversation
	a.conversation.Append(response.ToParam())

	a.log.InfoContext(ctx, "run completed",
		logArgs(ctx,
			"stop_reason", string(response.StopReason),
			"input_tokens", response.Usage.InputTokens,
			"output_tokens", response.Usage.OutputTokens,
		)...,
	)

	return nil
}

// buildRequest constructs a CompletionRequest from the Agent's config
// and current conversation state.
func (a *Agent) buildRequest() CompletionRequest {
	req := CompletionRequest{
		Messages:  a.conversation.Messages(),
		Model:     a.config.Model,
		MaxTokens: a.config.MaxTokens,
	}

	if a.config.System != "" {
		req.System = []anthropic.TextBlockParam{
			{Text: a.config.System},
		}
	}

	if tools := a.registry.Tools(); len(tools) > 0 {
		req.Tools = tools
	}

	if a.config.Temperature != nil {
		req.Temperature = a.config.Temperature
	}

	return req
}

// complete calls the Completer and streams events to the handler.
// Returns the accumulated Message on success.
func (a *Agent) complete(ctx context.Context, req CompletionRequest, handler EventHandler) (anthropic.Message, error) {
	ctx, span := startSpan(ctx, "agent.llm_call",
		attribute.String("agent.model", string(req.Model)),
		attribute.Int("agent.message_count", len(req.Messages)),
	)
	var callErr error
	defer func() { endSpan(span, callErr) }()

	a.log.DebugContext(ctx, "llm call started",
		logArgs(ctx, "message_count", len(req.Messages))...,
	)

	stream, err := a.completer.Complete(ctx, req)
	if err != nil {
		callErr = err
		return anthropic.Message{}, fmt.Errorf("completing: %w", err)
	}
	defer stream.Close()

	for stream.Next() {
		if handler != nil {
			handler(stream.Event())
		}
	}

	if err := stream.Err(); err != nil {
		callErr = err
		return anthropic.Message{}, fmt.Errorf("streaming: %w", err)
	}

	msg := stream.Message()

	// Send done event
	if handler != nil {
		handler(Event{Type: EventDone})
	}

	a.log.DebugContext(ctx, "llm call completed",
		logArgs(ctx,
			"stop_reason", string(msg.StopReason),
			"output_tokens", msg.Usage.OutputTokens,
		)...,
	)

	return msg, nil
}

// ErrMaxIterations is returned when the agentic loop exceeds the
// configured maximum iteration count.
var ErrMaxIterations = errors.New("maximum iterations exceeded")
