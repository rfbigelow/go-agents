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

	// Logger is the structured logger used by the Agent.
	// If nil, slog.Default() is used.
	Logger *slog.Logger
}

func (c Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}
