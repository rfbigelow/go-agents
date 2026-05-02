package agent

import (
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
)

// Config holds the configuration for an Agent.
type Config struct {
	// System is the system prompt sent with every LLM request.
	System string

	// Model specifies which Anthropic model to use.
	Model anthropic.Model

	// MaxTokens is the maximum number of tokens in each LLM response.
	MaxTokens int64

	// MaxIterations is the maximum number of agentic loop iterations
	// before the Agent terminates the run with an error.
	MaxIterations int

	// Temperature controls the sampling temperature for LLM responses.
	// nil uses the model's default.
	Temperature *float64

	// Thinking configures Anthropic's Extended Thinking feature.
	// nil omits the parameter from requests (S2.9).
	Thinking *ThinkingConfig

	// Effort configures the Anthropic Messages API output_config.effort
	// parameter. nil omits the parameter from requests (S2.16).
	Effort *string

	// Logger is the structured logger used by the Agent.
	// If nil, slog.Default() is used.
	Logger *slog.Logger
}

// ThinkingConfig describes the Extended Thinking configuration to send on
// each Completer request (S2.9). Type is a passed-through string; the
// library does not validate it against the targeted model. BudgetTokens
// is honored when Type is "enabled". Display is "summarized" or "omitted";
// when nil for "enabled" or "adaptive", the library defaults to "omitted".
type ThinkingConfig struct {
	Type         string
	BudgetTokens *int64
	Display      *string
}

func (c Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}
