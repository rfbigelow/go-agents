package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/rfbigelow/go-agents/agent"
)

// In-memory store of "sent" messages. Mutex-guarded because dispatch.go
// runs approved tools in parallel; this example only flags one tool as
// HITL but list_sent and a future approved send_email could race.
type sentMessage struct {
	ID      int    `json:"id"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"-"`
}

var (
	sentMu sync.Mutex
	sent   []sentMessage
	nextID int
)

const systemPrompt = `You are an email assistant with two tools: send_email (sends a message; requires human approval) and list_sent (lists messages already sent in this session). When the user asks you to send mail, call send_email with to/subject/body. If approval is denied, acknowledge it and ask whether they want to revise the message — do not retry a denied send without new instructions from the user.`

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	scanner := bufio.NewScanner(os.Stdin)

	registry := agent.NewToolRegistry()
	registry.SetApprovalCallback(approvalCallback(scanner))
	registerTools(registry)

	client := anthropic.NewClient()
	completer := agent.NewAnthropicCompleter(client)

	a := agent.NewAgent(completer, registry, agent.Config{
		System:    systemPrompt,
		Model:     anthropic.ModelClaudeSonnet4_5,
		MaxTokens: 1024,
		Logger:    logger,
	})

	fmt.Println("HITL chat (type 'quit' to exit). Suggested script:")
	fmt.Println(`  > send a friendly hello email to alice@example.com with subject "Hi"`)
	fmt.Println("    (when prompted: y)")
	fmt.Println("  > now send a similar one to bob@example.com")
	fmt.Println("    (when prompted: n)")
	fmt.Println("  > what got sent?")
	fmt.Println()
	fmt.Println("Note: slog output goes to stderr; assistant text goes to stdout.")
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

// approvalCallback returns a callback that prompts on stderr and reads
// the human's decision from the shared stdin scanner. The scanner is
// shared with the input loop; this is safe because Agent.Run is
// synchronous and blocks the input loop until it returns, so the two
// readers never overlap.
func approvalCallback(scanner *bufio.Scanner) agent.ApprovalCallback {
	return func(_ context.Context, c agent.ToolCall) (bool, error) {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, c.Input, "  ", "  "); err != nil {
			pretty.Write(c.Input)
		}
		fmt.Fprintf(os.Stderr, "\n[approval] tool=%s\n  %s\nApprove? [y/N]: ",
			c.Name, pretty.String())
		if !scanner.Scan() {
			return false, nil
		}
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return ans == "y" || ans == "yes", nil
	}
}

func registerTools(r *agent.ToolRegistry) {
	if err := r.Register(agent.Tool{
		Name:        "list_sent",
		Description: "Returns the messages already sent in this session as a JSON array of {id, to, subject}.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{},
		},
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			sentMu.Lock()
			snapshot := make([]sentMessage, len(sent))
			copy(snapshot, sent)
			sentMu.Unlock()
			out, err := json.Marshal(snapshot)
			if err != nil {
				return "", fmt.Errorf("marshal: %w", err)
			}
			return string(out), nil
		},
	}); err != nil {
		panic(err)
	}

	if err := r.Register(agent.Tool{
		Name:        "send_email",
		Description: "Sends an email message. Requires human approval before execution.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"to": map[string]any{
					"type":        "string",
					"description": "Recipient email address.",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Subject line.",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Message body.",
				},
			},
			Required: []string{"to", "subject", "body"},
		},
		HITL: true,
		Execute: func(_ context.Context, args json.RawMessage) (string, error) {
			var in struct {
				To      string `json:"to"`
				Subject string `json:"subject"`
				Body    string `json:"body"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}
			sentMu.Lock()
			nextID++
			id := nextID
			sent = append(sent, sentMessage{
				ID:      id,
				To:      in.To,
				Subject: in.Subject,
				Body:    in.Body,
			})
			sentMu.Unlock()
			return fmt.Sprintf("sent: id=%d to=%s", id, in.To), nil
		},
	}); err != nil {
		panic(err)
	}
}
