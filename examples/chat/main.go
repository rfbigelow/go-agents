package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/rfbigelow/go-agents/agent"
)

func main() {
	client := anthropic.NewClient()
	completer := agent.NewAnthropicCompleter(client)
	registry := agent.NewToolRegistry()

	a := agent.NewAgent(completer, registry, agent.Config{
		System:    "You are a helpful assistant. Be concise.",
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 1024,
	})

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Chat with Claude (type 'quit' to exit)")
	fmt.Println()

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "quit" {
			break
		}

		err := a.Run(context.Background(), input, func(e agent.Event) {
			if e.Type == agent.EventTextDelta {
				fmt.Print(e.Text)
			}
			if e.Type == agent.EventDone {
				fmt.Println()
				fmt.Println()
			}
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}
