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

const (
	ansiGray  = "\033[90m"
	ansiReset = "\033[0m"
)

func main() {
	model := chatModel()
	thinkingCfg := thinkingConfigFor(model)

	client := anthropic.NewClient()
	completer := agent.NewAnthropicCompleter(client)
	registry := agent.NewToolRegistry()

	a := agent.NewAgent(completer, registry, agent.Config{
		System:    "You are a helpful assistant. Be concise.",
		Model:     model,
		MaxTokens: 8192,
		Thinking:  thinkingCfg,
	})

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("Chat with Claude (%s, thinking: %s, type 'quit' to exit)\n\n", model, thinkingLabel(thinkingCfg))

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

		inThinking := false
		exitThinking := func() {
			if inThinking {
				fmt.Print(ansiReset + "\n")
				inThinking = false
			}
		}

		err := a.Run(context.Background(), input, func(e agent.Event) {
			switch e.Type {
			case agent.EventThinkingDelta:
				if !inThinking {
					fmt.Print(ansiGray + "thinking: ")
					inThinking = true
				}
				fmt.Print(e.Thinking)
			case agent.EventTextDelta:
				exitThinking()
				fmt.Print(e.Text)
			case agent.EventDone:
				exitThinking()
				fmt.Println()
				fmt.Println()
			}
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}

// chatModel returns the Anthropic model to use, honoring the CHAT_MODEL
// environment variable when set.
func chatModel() anthropic.Model {
	if m := os.Getenv("CHAT_MODEL"); m != "" {
		return anthropic.Model(m)
	}
	return anthropic.ModelClaudeSonnet4_6
}

// thinkingConfigFor returns the Extended Thinking configuration appropriate
// for the given model, or nil for models that do not support thinking
// (e.g., Haiku, Claude 3.x). Adaptive-capable models use adaptive (the only
// mode supported on Opus 4.7); older Claude 4.x models use enabled with a
// budget. Unknown models degrade to nil so the example does not surface
// a pass-through API error to readers running it for the first time.
// Display is set to "summarized" so the chat UI can render thinking text.
func thinkingConfigFor(model anthropic.Model) *agent.ThinkingConfig {
	display := "summarized"

	switch model {
	case anthropic.ModelClaudeOpus4_7,
		anthropic.ModelClaudeOpus4_6,
		anthropic.ModelClaudeSonnet4_6:
		return &agent.ThinkingConfig{
			Type:    "adaptive",
			Display: &display,
		}

	case anthropic.ModelClaudeOpus4_5,
		anthropic.ModelClaudeOpus4_5_20251101,
		anthropic.ModelClaudeSonnet4_5,
		anthropic.ModelClaudeSonnet4_5_20250929,
		anthropic.ModelClaudeOpus4_1,
		anthropic.ModelClaudeOpus4_1_20250805,
		anthropic.ModelClaudeOpus4_0,
		anthropic.ModelClaudeOpus4_20250514,
		anthropic.ModelClaudeSonnet4_0,
		anthropic.ModelClaudeSonnet4_20250514:
		budget := int64(2048)
		return &agent.ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: &budget,
			Display:      &display,
		}
	}

	return nil
}

// thinkingLabel returns a short banner-friendly label for a thinking config.
func thinkingLabel(cfg *agent.ThinkingConfig) string {
	if cfg == nil {
		return "off"
	}
	return cfg.Type
}
