package agent

import "fmt"

// Hook point identifiers used by hook error types.
const (
	hookPointPreLLMCall  = "pre_llm_call"
	hookPointPreToolUse  = "pre_tool_use"
	hookPointPostToolUse = "post_tool_use"
)

// HookError is the common interface satisfied by every hook-related
// error returned from Run (S2.10). Consumers can branch on the concrete
// type via errors.As to distinguish:
//
//   - *HookAbortError   — handler returned an Abort decision (intentional)
//   - *HookHandlerError — handler returned a non-nil error (malfunction)
//   - *HookPanicError   — handler panicked
type HookError interface {
	error

	// HookPoint identifies which hook point produced the error
	// ("pre_llm_call", "pre_tool_use", or "post_tool_use").
	HookPoint() string
}

// HookAbortError is returned when a hook handler returns an Abort
// decision. The Reason value is the handler's policy explanation; it is
// available via Unwrap so callers can errors.Is against sentinel reasons.
type HookAbortError struct {
	Hook   string
	Reason error
}

// Error implements error.
func (e *HookAbortError) Error() string {
	if e.Reason == nil {
		return fmt.Sprintf("hook %s aborted run", e.Hook)
	}
	return fmt.Sprintf("hook %s aborted run: %v", e.Hook, e.Reason)
}

// Unwrap returns the Reason so errors.Is can match sentinel values
// embedded by the handler.
func (e *HookAbortError) Unwrap() error { return e.Reason }

// HookPoint identifies which hook point aborted.
func (e *HookAbortError) HookPoint() string { return e.Hook }

// HookHandlerError is returned when a hook handler returned a non-nil
// error alongside its decision. The agent discards the decision; this
// represents a malfunction in the handler, distinct from an intentional
// Abort.
type HookHandlerError struct {
	Hook string
	Err  error
}

// Error implements error.
func (e *HookHandlerError) Error() string {
	return fmt.Sprintf("hook %s handler failed: %v", e.Hook, e.Err)
}

// Unwrap returns the underlying handler error.
func (e *HookHandlerError) Unwrap() error { return e.Err }

// HookPoint identifies which hook point failed.
func (e *HookHandlerError) HookPoint() string { return e.Hook }

// HookPanicError is returned when a hook handler panics. The recovered
// value is preserved verbatim; if it happens to be an error value,
// Unwrap exposes it so callers can errors.Is into it.
type HookPanicError struct {
	Hook      string
	Recovered any
}

// Error implements error.
func (e *HookPanicError) Error() string {
	return fmt.Sprintf("hook %s panicked: %v", e.Hook, e.Recovered)
}

// Unwrap exposes the recovered value when it satisfies the error
// interface, allowing errors.Is/errors.As to walk into it. Returns nil
// for non-error panics (e.g., string panics).
func (e *HookPanicError) Unwrap() error {
	if err, ok := e.Recovered.(error); ok {
		return err
	}
	return nil
}

// HookPoint identifies which hook point panicked.
func (e *HookPanicError) HookPoint() string { return e.Hook }
