package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/rfbigelow/go-agents/agent"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	registry := agent.NewToolRegistry()
	registerTools(registry)

	client := anthropic.NewClient()
	completer := agent.NewAnthropicCompleter(client)

	a := agent.NewAgent(completer, registry, agent.Config{
		System:    "You are a helpful assistant. Use the available tools when they help answer the question.",
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 1024,
		Logger:    logger,
	})

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Tool-use chat (type 'quit' to exit). Try: what time is it, and what's 17 * 23?")
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

func registerTools(r *agent.ToolRegistry) {
	if err := r.Register(agent.Tool{
		Name:        "get_current_time",
		Description: "Returns the current local time as an RFC3339 string.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return time.Now().Format(time.RFC3339), nil
		},
	}); err != nil {
		panic(err)
	}

	if err := r.Register(agent.Tool{
		Name:        "calculate",
		Description: "Evaluates a Go-syntax arithmetic expression (supports +, -, *, / and parentheses). Example: \"17 * 23\".",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "The arithmetic expression to evaluate.",
				},
			},
			Required: []string{"expression"},
		},
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Expression string `json:"expression"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			v, err := evalExpr(in.Expression)
			if err != nil {
				return "", err
			}
			return strconv.FormatFloat(v, 'f', -1, 64), nil
		},
	}); err != nil {
		panic(err)
	}
}

// evalExpr parses a Go-syntax arithmetic expression and evaluates it.
// Supports literals, the binary operators + - * /, unary -, and parens.
func evalExpr(s string) (float64, error) {
	expr, err := parser.ParseExpr(s)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	}
	return evalNode(expr)
}

func evalNode(n ast.Expr) (float64, error) {
	switch v := n.(type) {
	case *ast.BasicLit:
		switch v.Kind {
		case token.INT, token.FLOAT:
			return strconv.ParseFloat(v.Value, 64)
		}
		return 0, fmt.Errorf("unsupported literal kind %v", v.Kind)
	case *ast.ParenExpr:
		return evalNode(v.X)
	case *ast.UnaryExpr:
		x, err := evalNode(v.X)
		if err != nil {
			return 0, err
		}
		switch v.Op {
		case token.SUB:
			return -x, nil
		case token.ADD:
			return x, nil
		}
		return 0, fmt.Errorf("unsupported unary op %v", v.Op)
	case *ast.BinaryExpr:
		l, err := evalNode(v.X)
		if err != nil {
			return 0, err
		}
		r, err := evalNode(v.Y)
		if err != nil {
			return 0, err
		}
		switch v.Op {
		case token.ADD:
			return l + r, nil
		case token.SUB:
			return l - r, nil
		case token.MUL:
			return l * r, nil
		case token.QUO:
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return l / r, nil
		}
		return 0, fmt.Errorf("unsupported binary op %v", v.Op)
	}
	return 0, fmt.Errorf("unsupported expression node %T", n)
}
