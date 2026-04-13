# go-agents

A lightweight Go library for building LLM-based agents with the Anthropic API.

## Overview

go-agents provides reusable infrastructure for agentic workflows so you can
focus on domain-specific behavior rather than plumbing. The library manages the
agentic loop (LLM calls, tool dispatch, streaming) and conversation state,
instrumented with OpenTelemetry tracing and structured logging.

Core components:

- **Agent** -- drives the agentic loop, coordinates tool execution and
  conversation history
- **Completer** -- stateless adapter bridging to the Anthropic Go SDK
- **Tool Registry** -- manages tool definitions and dispatch
- **Conversation State** -- maintains message history across turns

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/rfbigelow/go-agents/agent"
)

func main() {
	client := anthropic.NewClient() // reads ANTHROPIC_API_KEY from env
	completer := agent.NewAnthropicCompleter(client)
	registry := agent.NewToolRegistry()

	a := agent.NewAgent(completer, registry, agent.Config{
		System:    "You are a helpful assistant.",
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 1024,
	})

	err := a.Run(context.Background(), "Hello!", func(e agent.Event) {
		if e.Type == agent.EventTextDelta {
			fmt.Print(e.Text)
		}
	})
	if err != nil {
		panic(err)
	}
	fmt.Println()
}
```

## Installation

```
go get github.com/rfbigelow/go-agents
```

Requires Go 1.25+ and an Anthropic API key.

## Running the Example

```
export ANTHROPIC_API_KEY=sk-ant-...
go run ./examples/chat/
```

## Project Status

M1 (Basic Conversation) is implemented: streaming completions, conversation
state management, and observability (OTEL tracing + slog logging).

Planned milestones: Tool Use (M2), Human-in-the-Loop (M3), Extended
Thinking (M4), Deterministic Logic (M5), Example Application (M6).

See [requirements/](requirements/README.md) for the full PEGS requirements.

## Dependencies

- [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go)
- [OpenTelemetry Trace API](https://pkg.go.dev/go.opentelemetry.io/otel/trace)
- Go standard library (slog, context)

## License

MIT
