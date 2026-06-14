package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/anthropics/anthropic-sdk-go"
)

// sessionIDPrefix is the trailer a multi-turn sub-agent appends to its result
// to carry the session handle (S2.11). It is referenced both when emitting the
// trailer and when documenting the convention in the tool description, so the
// two cannot drift. The exact "sessionId: <id>" form is what the model is told
// to parse and resupply via the session_id argument.
const sessionIDPrefix = "sessionId: "

// defaultSubAgentMaxTokens is used when SubAgentDefinition.MaxTokens is zero,
// so the common case needs no token configuration.
const defaultSubAgentMaxTokens = 1024

// SubAgentDefinition declares a sub-agent and is compiled into a Tool by
// NewSubAgentTool (S2.11, S3.5). The produced Tool runs a separate agent loop
// with its own conversation state and returns the sub-agent's final message as
// the tool result.
type SubAgentDefinition struct {
	// Name is the tool name surfaced to the parent LLM. Required.
	Name string

	// Description tells the parent LLM when to invoke the sub-agent.
	Description string

	// System is the sub-agent's system prompt.
	System string

	// Model is the sub-agent's model. Required.
	Model anthropic.Model

	// MaxTokens caps each sub-agent LLM response. Defaults to 1024 when zero.
	MaxTokens int64

	// MaxIterations caps the sub-agent's agent loop. Zero uses the Agent
	// default (defaultMaxIterations).
	MaxIterations int

	// Tools is the sub-agent's tool registry (its tool subset). nil means the
	// sub-agent has no tools. The registry's approval callback is replaced at
	// compile time by the propagation bridge (see Approval).
	Tools *ToolRegistry

	// MultiTurn opts the sub-agent into multi-turn operation. When true, the
	// live sub-agent instance is retained across invocations and addressed by
	// a session handle returned in the result; when false (default), every
	// invocation runs a fresh sub-agent (S2.11).
	MultiTurn bool

	// Forward, when true, forwards the sub-agent's streaming events to the
	// parent's event handler, tagged with the sub-agent's name and nesting
	// depth (S2.3). Default (false) keeps the sub-agent's stream isolated.
	Forward bool

	// Observer, when non-nil, receives the sub-agent's streaming events on a
	// dedicated sink regardless of Forward, so the application can render the
	// sub-agent's stream independently (S2.11).
	Observer EventHandler

	// Approval, when non-nil, gates the sub-agent's HITL-flagged tools instead
	// of the propagated parent approval callback (S2.8). When nil, the parent's
	// approval callback governs the sub-agent's HITL tools.
	Approval ApprovalCallback

	// Thinking, Temperature, and Effort mirror the corresponding Config fields
	// for the sub-agent. nil omits them.
	Thinking    *ThinkingConfig
	Temperature *float64
	Effort      *string
}

// ForwardEvents returns an EventHandler that tags each event with the given
// sub-agent name and nesting depth (S2.3) before delivering it to parent. It
// returns nil when parent is nil (a fully isolated stream). The parent handler
// supplied to a sub-agent tool via SubAgentContext is already serialized, so
// parallel sub-agents forwarding to a shared handler never invoke it
// concurrently (S2.11).
func ForwardEvents(parent EventHandler, name string, depth int) EventHandler {
	if parent == nil {
		return nil
	}
	return func(e Event) {
		e.AgentName = name
		e.Depth = depth
		parent(e)
	}
}

// combineHandlers returns a single EventHandler that fans an event out to each
// non-nil handler in order, or nil when none are supplied.
func combineHandlers(handlers ...EventHandler) EventHandler {
	active := handlers[:0]
	for _, h := range handlers {
		if h != nil {
			active = append(active, h)
		}
	}
	if len(active) == 0 {
		return nil
	}
	if len(active) == 1 {
		return active[0]
	}
	hs := append([]EventHandler(nil), active...)
	return func(e Event) {
		for _, h := range hs {
			h(e)
		}
	}
}

// subAgentInput is the decoded tool input for a sub-agent tool.
type subAgentInput struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id"`
}

// sessionEntry holds a retained multi-turn sub-agent instance and serializes
// concurrent invocations that target the same session.
type sessionEntry struct {
	mu    sync.Mutex
	agent *Agent
}

// sessionStore retains multi-turn sub-agent instances keyed by session handle.
type sessionStore struct {
	mu      sync.Mutex
	entries map[string]*sessionEntry
	counter atomic.Int64
}

func (s *sessionStore) getOrCreate(id string, mk func() *Agent) (string, *sessionEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != "" {
		if e, ok := s.entries[id]; ok {
			return id, e
		}
	}
	if id == "" {
		id = "s" + strconv.FormatInt(s.counter.Add(1), 10)
	}
	e := &sessionEntry{agent: mk()}
	s.entries[id] = e
	return id, e
}

// NewSubAgentTool compiles a SubAgentDefinition into a Tool (S2.11, S3.5). The
// completer is shared by every sub-agent instance the tool runs. The returned
// Tool's input schema accepts the sub-agent prompt (plus an optional
// session_id for multi-turn sub-agents); its result is the sub-agent's final
// message, with the session handle appended for multi-turn sub-agents.
//
// The parent's event stream and approval callback reach the sub-agent through
// the context propagated by Agent.Run, so the tool inherits the parent's HITL
// gate and (when Forward is set) streaming without extra wiring. A sub-agent
// tool invoked from within a sub-agent returns an error result, enforcing the
// maximum nesting depth of one.
func NewSubAgentTool(completer Completer, def SubAgentDefinition) (Tool, error) {
	if completer == nil {
		return Tool{}, fmt.Errorf("sub-agent tool %q: completer is nil", def.Name)
	}
	if def.Name == "" {
		return Tool{}, ErrEmptyToolName
	}
	if def.Model == "" {
		return Tool{}, fmt.Errorf("sub-agent tool %q: model is empty", def.Name)
	}

	registry := def.Tools
	if registry == nil {
		registry = NewToolRegistry()
	}
	// Replace the sub-agent registry's approval with a stable bridge: it uses
	// the definition's own callback when set, otherwise the parent approval
	// propagated on the context. Set once here (single-goroutine setup) so no
	// per-call mutation races on the shared registry (S2.8).
	registry.SetApprovalCallback(func(ctx context.Context, call ToolCall) (bool, error) {
		if def.Approval != nil {
			return def.Approval(ctx, call)
		}
		if cb, ok := inheritedApprovalFrom(ctx); ok && cb != nil {
			return cb(ctx, call)
		}
		// A HITL tool must not run ungated.
		return false, nil
	})

	maxTokens := def.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultSubAgentMaxTokens
	}
	childConfig := Config{
		System:        def.System,
		Model:         def.Model,
		MaxTokens:     maxTokens,
		MaxIterations: def.MaxIterations,
		Temperature:   def.Temperature,
		Thinking:      def.Thinking,
		Effort:        def.Effort,
	}

	store := &sessionStore{entries: make(map[string]*sessionEntry)}

	props := map[string]any{
		"prompt": map[string]any{
			"type":        "string",
			"description": "The task or message for the sub-agent.",
		},
	}
	description := def.Description
	if def.MultiTurn {
		props["session_id"] = map[string]any{
			"type": "string",
			"description": "Continue an existing session by passing the id from " +
				"the \"" + sessionIDPrefix + "<id>\" line at the end of a " +
				"previous result. Omit to start a new session.",
		}
		// The library owns the result trailer, so it documents the output
		// convention in the description (tool-use output-format guidance);
		// callers need not write it into def.Description.
		description += "\n\nMulti-turn: the result ends with a line \"" +
			sessionIDPrefix + "<id>\". To continue this sub-agent on a later " +
			"call, pass that id as the session_id argument; omit session_id to " +
			"start a new session."
	}

	tool := Tool{
		Name:        def.Name,
		Description: description,
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: props,
			Required:   []string{"prompt"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			var in subAgentInput
			if err := json.Unmarshal(args, &in); err != nil {
				return "", fmt.Errorf("invalid sub-agent arguments: %w", err)
			}
			if in.Prompt == "" {
				return "", fmt.Errorf("sub-agent %q: prompt is required", def.Name)
			}

			parentHandler, parentApproval, parentDepth := SubAgentContext(ctx)
			if parentDepth >= 1 {
				// Enforce maximum nesting depth of one (S2.11).
				return "", fmt.Errorf("sub-agent %q cannot spawn a sub-agent: maximum nesting depth of one (S2.11)", def.Name)
			}

			// Build the sub-agent's stream: a final-message accumulator, the
			// optional dedicated observer, and optional attributed forwarding
			// to the parent's (serialized) handler.
			var final, cur strings.Builder
			accumulate := func(e Event) {
				switch e.Type {
				case EventTextDelta:
					cur.WriteString(e.Text)
				case EventDone:
					final.Reset()
					final.WriteString(cur.String())
					cur.Reset()
				}
			}
			var forward EventHandler
			if def.Forward {
				forward = ForwardEvents(parentHandler, def.Name, parentDepth+1)
			}
			childHandler := combineHandlers(accumulate, def.Observer, forward)

			// childCtx carries the propagated parent approval (read by the
			// bridge) and the incremented depth for the child run. Agent.Run
			// overwrites the runtime's handler/approval with the child's own;
			// the inherited approval rides a separate key that survives.
			childCtx := ctx
			if parentApproval != nil {
				childCtx = withInheritedApproval(childCtx, parentApproval)
			}
			childCtx = withSubAgentRuntime(childCtx, subAgentRuntime{depth: parentDepth + 1})

			childCtx, span := startSpan(childCtx, "agent.sub_agent."+def.Name)

			var runErr error
			if def.MultiTurn {
				id, entry := store.getOrCreate(in.SessionID, func() *Agent {
					return NewAgent(completer, registry, childConfig)
				})
				entry.mu.Lock()
				runErr = entry.agent.Run(childCtx, in.Prompt, childHandler)
				entry.mu.Unlock()
				endSpan(span, runErr)
				if runErr != nil {
					return "", runErr
				}
				result := final.String()
				return fmt.Sprintf("%s\n\n%s%s", result, sessionIDPrefix, id), nil
			}

			child := NewAgent(completer, registry, childConfig)
			runErr = child.Run(childCtx, in.Prompt, childHandler)
			endSpan(span, runErr)
			if runErr != nil {
				return "", runErr
			}
			return final.String(), nil
		},
	}
	return tool, nil
}
