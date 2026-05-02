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

## Why This Exists

go-agents is both a working library and a deliberate exercise in applying
requirements engineering rigor to agent development. The code is intended
to be useful on its own terms, but the project is also an experiment in
whether a disciplined, PEGS-structured requirements process produces
better design decisions than jumping straight to implementation — a
question that feels especially sharp for LLM-based systems, where the
problem space is fluid and conventions are still forming.

Readers interested in the methodology rather than the API should start
with [requirements/README.md](requirements/README.md), which documents
the PEGS structure used here and links to the four requirements books.

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

## Running the Examples

```
export ANTHROPIC_API_KEY=sk-ant-...
go run ./examples/chat/       # basic streaming chat
go run ./examples/tool-use/   # tool use: current time + calculator
go run ./examples/hitl/       # tool use with human approval gate
```

## Project Status

M1 (Basic Conversation), M2 (Tool Use), M3 (HITL Example), and M4
(Extended Thinking) are implemented: streaming completions, conversation
state management, tool registration, parallel tool dispatch with a
working human approval gate (see `examples/hitl/`), Extended Thinking
with adaptive and enabled modes plus `output_config.effort` (see
`examples/chat/`), and observability (OTEL tracing + slog logging)
across LLM calls, tool-dispatch batches, and individual tool executions.

Planned milestones: Deterministic Logic (M5), Example Application (M6).

See [requirements/](requirements/README.md) for the full PEGS requirements.

## Dependencies

- [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go)
- [OpenTelemetry Trace API](https://pkg.go.dev/go.opentelemetry.io/otel/trace)
- Go standard library (slog, context)

## License

MIT
