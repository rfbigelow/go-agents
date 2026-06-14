// Command sub-agent demonstrates sub-agent composition (S2.11): a parent agent
// that delegates to sub-agents exposed as tools. It shows all four facets of
// the feature:
//
//   - a one-shot sub-agent (math_helper) with its own tool;
//   - a multi-turn sub-agent (notes_keeper) the parent can steer across turns
//     via a session handle;
//   - optional attributed stream forwarding (math_helper.Forward), so the
//     sub-agent's streamed output surfaces on the parent's stream tagged with
//     its name and nesting depth;
//   - HITL approval propagation (S2.8): the parent's approval callback governs
//     the secure_agent sub-agent's HITL-flagged tool.
//
// Run with ANTHROPIC_API_KEY set. Try prompts like:
//
//	what is 17 * 23? (delegates to math_helper)
//	remember that my project is called Orion, then later: what is my project called?
//	delete the file /tmp/report.txt (delegates to secure_agent, asks approval)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/rfbigelow/go-agents/agent"
)

const (
	// indent prefixes every line of a sub-agent's output so the whole block
	// reads as nested under its parent.
	indent = "  "
	// grey/reset shade thinking output so reasoning is visually distinct from
	// the model's regular response.
	grey  = "\033[90m"
	reset = "\033[0m"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	client := anthropic.NewClient()
	completer := agent.NewAnthropicCompleter(client)
	scanner := bufio.NewScanner(os.Stdin)

	registry := agent.NewToolRegistry()
	// The parent's approval callback propagates to sub-agents' HITL tools
	// (S2.8). It must be installed before registering anything HITL-flagged.
	registry.SetApprovalCallback(approvalCallback(scanner))
	registerSubAgents(completer, registry)

	a := agent.NewAgent(completer, registry, agent.Config{
		System: "You are an orchestrator. Delegate math to the math_helper " +
			"sub-agent, note-taking to the notes_keeper sub-agent, and any " +
			"destructive file operation to the secure_agent sub-agent.",
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 1024,
		Logger:    logger,
	})

	fmt.Println("Sub-agent demo (type 'quit' to exit).")
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

		// block identifies who owns the current output line and whether it is
		// reasoning ("<owner>/think") or response ("<owner>/text"), where owner
		// is "parent" or a sub-agent name. Tracking it lets us print one header
		// per contiguous block and terminate the line on each EventDone, so a
		// following log entry (sub-agents log to stderr at Info) starts fresh
		// instead of butting against the last output.
		block := ""
		err := a.Run(context.Background(), input, func(e agent.Event) {
			var content string
			thinking := false
			switch e.Type {
			case agent.EventTextDelta:
				content = e.Text
			case agent.EventThinkingDelta:
				content, thinking = e.Thinking, true
			case agent.EventDone:
				if block != "" {
					fmt.Println()
					block = ""
				}
				return
			default:
				return
			}

			// Forwarded sub-agent output carries attribution; tag it so it is
			// distinguishable from the parent's own output (S2.3).
			sub := e.AgentName != ""
			owner := "parent"
			if sub {
				owner = e.AgentName
			}
			key := owner + "/text"
			if thinking {
				key = owner + "/think"
			}
			if block != key {
				if block != "" {
					fmt.Println()
				}
				if sub {
					fmt.Print(indent + "[" + owner + "] ")
				}
				block = key
			}

			// Indent continuation lines so the whole sub-agent block is nested,
			// and shade reasoning grey so it reads apart from the response.
			if sub {
				content = strings.ReplaceAll(content, "\n", "\n"+indent)
			}
			if thinking {
				content = grey + content + reset
			}
			fmt.Print(content)
		})
		fmt.Println()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}

func registerSubAgents(completer agent.Completer, parent *agent.ToolRegistry) {
	// math_helper: a one-shot sub-agent with its own multiply tool. Forward is
	// enabled so its streamed reasoning surfaces on the parent's stream.
	mathTools := agent.NewToolRegistry()
	mustRegister(mathTools, agent.Tool{
		Name:        "multiply",
		Description: "Multiplies two numbers and returns the product.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
			Required: []string{"a", "b"},
		},
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			var in struct {
				A float64 `json:"a"`
				B float64 `json:"b"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			return strconv.FormatFloat(in.A*in.B, 'f', -1, 64), nil
		},
	})
	mustRegisterSubAgent(parent, completer, agent.SubAgentDefinition{
		Name:        "math_helper",
		Description: "Performs arithmetic. Give it a natural-language math question.",
		System:      "You are a meticulous calculator. Use the multiply tool for products.",
		Model:       anthropic.ModelClaudeSonnet4_5,
		Tools:       mathTools,
		Forward:     true,
	})

	// notes_keeper: a multi-turn sub-agent. The first call returns a sessionId;
	// the parent passes it back to continue the same conversation (S2.11).
	mustRegisterSubAgent(parent, completer, agent.SubAgentDefinition{
		Name: "notes_keeper",
		Description: "Remembers and recalls notes across turns. To continue an " +
			"existing notes session, pass its sessionId.",
		System:    "You keep concise notes for the user and recall them on request.",
		Model:     anthropic.ModelClaudeSonnet4_5,
		MultiTurn: true,
	})

	// secure_agent: a sub-agent whose destructive tool is HITL-flagged. The
	// parent's approval callback (propagated) gates it.
	secureTools := agent.NewToolRegistry()
	// A placeholder is required to register a HITL tool; NewSubAgentTool
	// replaces it with the parent-propagation bridge.
	secureTools.SetApprovalCallback(func(context.Context, agent.ToolCall) (bool, error) {
		return false, nil
	})
	mustRegister(secureTools, agent.Tool{
		Name:        "delete_file",
		Description: "Deletes a file at the given path. Requires human approval.",
		HITL:        true,
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"path": map[string]any{"type": "string"},
			},
			Required: []string{"path"},
		},
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			// Simulated — this demo does not touch the filesystem.
			return fmt.Sprintf("deleted %s (simulated)", in.Path), nil
		},
	})
	mustRegisterSubAgent(parent, completer, agent.SubAgentDefinition{
		Name:        "secure_agent",
		Description: "Performs destructive file operations under human approval.",
		System:      "You carry out file operations the user requests.",
		Model:       anthropic.ModelClaudeSonnet4_5,
		Tools:       secureTools,
	})
}

// approvalCallback prompts on stderr and reads a y/N decision from the shared
// scanner. Run is synchronous, so sharing the scanner with the main loop is
// safe. The callback can tell a sub-agent call from a top-level one via the
// propagated nesting depth (S2.11).
func approvalCallback(scanner *bufio.Scanner) agent.ApprovalCallback {
	return func(ctx context.Context, c agent.ToolCall) (bool, error) {
		_, _, depth := agent.SubAgentContext(ctx)
		origin := "top-level"
		if depth > 0 {
			origin = fmt.Sprintf("sub-agent (depth %d)", depth)
		}
		fmt.Fprintf(os.Stderr, "\n[approval] %s tool=%s\n  %s\nApprove? [y/N]: ",
			origin, c.Name, string(c.Input))
		if !scanner.Scan() {
			return false, nil
		}
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return ans == "y" || ans == "yes", nil
	}
}

func mustRegister(r *agent.ToolRegistry, t agent.Tool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

func mustRegisterSubAgent(parent *agent.ToolRegistry, completer agent.Completer, def agent.SubAgentDefinition) {
	tool, err := agent.NewSubAgentTool(completer, def)
	if err != nil {
		panic(err)
	}
	mustRegister(parent, tool)
}
