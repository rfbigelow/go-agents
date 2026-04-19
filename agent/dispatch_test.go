package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// discardLogger returns a slog logger that drops all output — suitable for
// tests that do not assert on log content.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func mustRegister(t *testing.T, r *ToolRegistry, tool Tool) {
	t.Helper()
	if err := r.Register(tool); err != nil {
		t.Fatalf("register %q: %v", tool.Name, err)
	}
}

func TestDispatch_UnknownTool(t *testing.T) {
	r := NewToolRegistry()
	results := r.dispatch(context.Background(), []ToolCall{
		{ID: "call_1", Name: "missing"},
	}, discardLogger())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Fatalf("expected IsError=true for unknown tool")
	}
	if !strings.Contains(results[0].Content, `unknown tool "missing"`) {
		t.Fatalf("unexpected content: %q", results[0].Content)
	}
}

func TestDispatch_Success(t *testing.T) {
	r := NewToolRegistry()
	mustRegister(t, r, Tool{
		Name: "echo",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "ok", nil
		},
	})

	results := r.dispatch(context.Background(), []ToolCall{
		{ID: "call_1", Name: "echo"},
	}, discardLogger())

	if results[0].IsError {
		t.Fatalf("unexpected error result: %q", results[0].Content)
	}
	if results[0].Content != "ok" {
		t.Fatalf("expected content 'ok', got %q", results[0].Content)
	}
	if results[0].ID != "call_1" {
		t.Fatalf("expected ID 'call_1', got %q", results[0].ID)
	}
}

func TestDispatch_ParallelExecution(t *testing.T) {
	r := NewToolRegistry()

	// Both tools block on the same barrier. If execution is serial, the
	// second goroutine never reaches the barrier and we deadlock → test
	// fails via timeout.
	barrier := make(chan struct{})
	var reached sync.WaitGroup
	reached.Add(2)

	blocker := func(_ context.Context, _ json.RawMessage) (string, error) {
		reached.Done()
		<-barrier
		return "done", nil
	}
	mustRegister(t, r, Tool{Name: "a", Execute: blocker})
	mustRegister(t, r, Tool{Name: "b", Execute: blocker})

	done := make(chan []toolResult, 1)
	go func() {
		done <- r.dispatch(context.Background(), []ToolCall{
			{ID: "1", Name: "a"},
			{ID: "2", Name: "b"},
		}, discardLogger())
	}()

	// Wait for both goroutines to reach the barrier — proves parallelism.
	waitCh := make(chan struct{})
	go func() { reached.Wait(); close(waitCh) }()
	select {
	case <-waitCh:
	case <-time.After(time.Second):
		t.Fatal("tools did not run in parallel (both goroutines never reached the barrier)")
	}

	close(barrier)
	results := <-done
	if len(results) != 2 || results[0].Content != "done" || results[1].Content != "done" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestDispatch_SiblingIsolation(t *testing.T) {
	r := NewToolRegistry()
	mustRegister(t, r, Tool{
		Name: "ok",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "success", nil
		},
	})
	mustRegister(t, r, Tool{
		Name: "bad",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "", errors.New("kaboom")
		},
	})

	results := r.dispatch(context.Background(), []ToolCall{
		{ID: "a", Name: "ok"},
		{ID: "b", Name: "bad"},
		{ID: "c", Name: "ok"},
	}, discardLogger())

	if results[0].IsError || results[0].Content != "success" {
		t.Fatalf("result 0: %+v", results[0])
	}
	if !results[1].IsError || results[1].Content != "kaboom" {
		t.Fatalf("result 1: %+v", results[1])
	}
	if results[2].IsError || results[2].Content != "success" {
		t.Fatalf("result 2: %+v", results[2])
	}
}

func TestDispatch_PanicRecovered(t *testing.T) {
	r := NewToolRegistry()
	mustRegister(t, r, Tool{
		Name: "boom",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			panic("tool gone wild")
		},
	})
	mustRegister(t, r, Tool{
		Name: "ok",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "fine", nil
		},
	})

	results := r.dispatch(context.Background(), []ToolCall{
		{ID: "1", Name: "boom"},
		{ID: "2", Name: "ok"},
	}, discardLogger())

	if !results[0].IsError {
		t.Fatalf("panic result should be IsError: %+v", results[0])
	}
	if !strings.Contains(results[0].Content, "tool gone wild") {
		t.Fatalf("unexpected panic content: %q", results[0].Content)
	}
	if results[1].IsError || results[1].Content != "fine" {
		t.Fatalf("sibling should succeed: %+v", results[1])
	}
}

type ctxKey string

func TestDispatch_ContextInherited(t *testing.T) {
	r := NewToolRegistry()
	const key ctxKey = "canary"
	var seen any
	mustRegister(t, r, Tool{
		Name: "peek",
		Execute: func(ctx context.Context, _ json.RawMessage) (string, error) {
			seen = ctx.Value(key)
			return "", nil
		},
	})

	ctx := context.WithValue(context.Background(), key, "hello")
	_ = r.dispatch(ctx, []ToolCall{{ID: "1", Name: "peek"}}, discardLogger())
	if seen != "hello" {
		t.Fatalf("expected canary 'hello', got %v", seen)
	}
}

func TestDispatch_HITLApproved(t *testing.T) {
	r := NewToolRegistry()
	var executed bool
	r.SetApprovalCallback(func(_ context.Context, _ ToolCall) (bool, error) {
		return true, nil
	})
	mustRegister(t, r, Tool{
		Name: "sensitive",
		HITL: true,
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			executed = true
			return "did the thing", nil
		},
	})

	results := r.dispatch(context.Background(), []ToolCall{{ID: "1", Name: "sensitive"}}, discardLogger())

	if !executed {
		t.Fatal("expected tool to execute after approval")
	}
	if results[0].IsError || results[0].Content != "did the thing" {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}

func TestDispatch_HITLDenied(t *testing.T) {
	r := NewToolRegistry()
	var executed bool
	r.SetApprovalCallback(func(_ context.Context, _ ToolCall) (bool, error) {
		return false, nil
	})
	mustRegister(t, r, Tool{
		Name: "sensitive",
		HITL: true,
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			executed = true
			return "", nil
		},
	})

	results := r.dispatch(context.Background(), []ToolCall{{ID: "1", Name: "sensitive"}}, discardLogger())

	if executed {
		t.Fatal("denied tool must not execute")
	}
	if !results[0].IsError {
		t.Fatalf("expected IsError for denied call, got %+v", results[0])
	}
	if !strings.Contains(results[0].Content, "denied") {
		t.Fatalf("expected denial message, got %q", results[0].Content)
	}
}

func TestDispatch_MixedBatchOrdering(t *testing.T) {
	r := NewToolRegistry()
	r.SetApprovalCallback(func(_ context.Context, call ToolCall) (bool, error) {
		// Approve only "approved_hitl", deny others.
		return call.Name == "approved_hitl", nil
	})
	mustRegister(t, r, Tool{
		Name: "plain",
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "plain-ok", nil
		},
	})
	mustRegister(t, r, Tool{
		Name: "approved_hitl",
		HITL: true,
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "hitl-ok", nil
		},
	})
	mustRegister(t, r, Tool{
		Name: "denied_hitl",
		HITL: true,
		Execute: func(_ context.Context, _ json.RawMessage) (string, error) {
			t.Fatal("denied tool ran")
			return "", nil
		},
	})

	results := r.dispatch(context.Background(), []ToolCall{
		{ID: "a", Name: "unknown"},
		{ID: "b", Name: "plain"},
		{ID: "c", Name: "approved_hitl"},
		{ID: "d", Name: "denied_hitl"},
	}, discardLogger())

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if results[0].ID != "a" || !results[0].IsError || !strings.Contains(results[0].Content, `unknown tool "unknown"`) {
		t.Fatalf("slot 0 (unknown): %+v", results[0])
	}
	if results[1].ID != "b" || results[1].IsError || results[1].Content != "plain-ok" {
		t.Fatalf("slot 1 (plain): %+v", results[1])
	}
	if results[2].ID != "c" || results[2].IsError || results[2].Content != "hitl-ok" {
		t.Fatalf("slot 2 (approved): %+v", results[2])
	}
	if results[3].ID != "d" || !results[3].IsError || !strings.Contains(results[3].Content, "denied") {
		t.Fatalf("slot 3 (denied): %+v", results[3])
	}
}
