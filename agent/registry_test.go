package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func noopTool(_ context.Context, _ json.RawMessage) (string, error) {
	return "", nil
}

func TestRegistry_Register_EmptyName(t *testing.T) {
	r := NewToolRegistry()
	err := r.Register(Tool{Execute: noopTool})
	if !errors.Is(err, ErrEmptyToolName) {
		t.Fatalf("expected ErrEmptyToolName, got %v", err)
	}
}

func TestRegistry_Register_NilExecute(t *testing.T) {
	r := NewToolRegistry()
	err := r.Register(Tool{Name: "x"})
	if !errors.Is(err, ErrNilToolFunc) {
		t.Fatalf("expected ErrNilToolFunc, got %v", err)
	}
}

func TestRegistry_Register_DuplicateName(t *testing.T) {
	r := NewToolRegistry()
	if err := r.Register(Tool{Name: "dup", Execute: noopTool}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := r.Register(Tool{Name: "dup", Execute: noopTool})
	if !errors.Is(err, ErrDuplicateTool) {
		t.Fatalf("expected ErrDuplicateTool, got %v", err)
	}
}

func TestRegistry_Register_HITLRequiresCallback(t *testing.T) {
	r := NewToolRegistry()
	err := r.Register(Tool{Name: "dangerous", Execute: noopTool, HITL: true})
	if !errors.Is(err, ErrNoApprovalCallback) {
		t.Fatalf("expected ErrNoApprovalCallback, got %v", err)
	}

	r.SetApprovalCallback(func(_ context.Context, _ ToolCall) (bool, error) {
		return true, nil
	})
	if err := r.Register(Tool{Name: "dangerous", Execute: noopTool, HITL: true}); err != nil {
		t.Fatalf("register after callback set: %v", err)
	}
}

func TestRegistry_Tools_EmptyReturnsNil(t *testing.T) {
	r := NewToolRegistry()
	if tools := r.Tools(); tools != nil {
		t.Fatalf("expected nil, got %v", tools)
	}
}

func TestRegistry_Tools_BuildsUnionParams(t *testing.T) {
	r := NewToolRegistry()
	if err := r.Register(Tool{
		Name:        "echo",
		Description: "Echoes its input",
		Execute:     noopTool,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(Tool{
		Name:    "silent",
		Execute: noopTool,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	tools := r.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Registration order preserved.
	if tools[0].OfTool == nil || tools[0].OfTool.Name != "echo" {
		t.Fatalf("expected first tool 'echo', got %+v", tools[0].OfTool)
	}
	if tools[1].OfTool == nil || tools[1].OfTool.Name != "silent" {
		t.Fatalf("expected second tool 'silent', got %+v", tools[1].OfTool)
	}

	if d := tools[0].OfTool.Description; !d.Valid() || d.Value != "Echoes its input" {
		t.Fatalf("expected echo description 'Echoes its input', got valid=%v value=%q", d.Valid(), d.Value)
	}
	// Empty description stays unset.
	if d := tools[1].OfTool.Description; d.Valid() {
		t.Fatalf("expected silent to have no description, got %q", d.Value)
	}
}
