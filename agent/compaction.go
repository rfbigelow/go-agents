package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"go.opentelemetry.io/otel/attribute"
)

// CompactionRequest carries what a CompactionStrategy needs to produce a
// compacted history (S3.6, S2.21): the history to consider, plus the
// Agent's Completer and model configuration for summarizing strategies.
type CompactionRequest struct {
	History   []anthropic.MessageParam
	Completer Completer
	Model     anthropic.Model
	MaxTokens int64
}

// CompactionResult is a strategy's proposal: replace the first Cut
// messages of the supplied history with Replacement. Cut of zero means
// no compaction. Usage reports any LLM call the strategy made (a
// summarizing strategy's own call, S2.21); it counts toward the
// conversation's token usage (S2.20).
type CompactionResult struct {
	Cut         int
	Replacement []anthropic.MessageParam
	Usage       anthropic.Usage
}

// CompactionStrategy is the extension point for conversation compaction
// (S3.6). A strategy examines the supplied history and proposes a
// shorter replacement for a prefix of it, cut at a boundary that
// preserves the protocol invariants (S2.6, S2.15): never separating a
// tool_use from its tool_result and never dropping the thinking blocks
// of a retained tool-use turn (S2.9). The library validates the
// composed result before committing and rejects invariant violations.
type CompactionStrategy interface {
	// Compact proposes a compaction of req.History. Returning a zero
	// Cut means no compaction is warranted.
	Compact(ctx context.Context, req CompactionRequest) (CompactionResult, error)
	// Committed reports whether the strategy's output must be committed
	// to the conversation history (S2.18). Non-deterministic or
	// expensive strategies (LLM summarization) must return true; a
	// pure, deterministic strategy whose output is stable across
	// consecutive turns may return false to be applied transiently to
	// the outgoing request without mutating committed state.
	Committed() bool
}

// ArchiveFunc receives the replaced prefix of a committed compaction in
// the SDK-native message representation before it is discarded (S2.18),
// so the application can persist it externally for lossless resume
// (S2.15). External persistence is the application's responsibility.
type ArchiveFunc func(prefix []anthropic.MessageParam)

// CompactionConfig is the Agent's compaction configuration (S2.18). The
// zero value disables compaction entirely: no strategy, so no trigger
// fires and the full history is retained (the library's default).
type CompactionConfig struct {
	// Strategy transforms the history when a trigger fires (S2.19).
	// Nil disables compaction.
	Strategy CompactionStrategy

	// TokenThreshold is the proactive trigger (S2.19) as an absolute
	// input-side token count: once the most recent LLM call's input
	// tokens (input + cache creation + cache read) reach it, the
	// strategy is applied before the next call. Zero disables the
	// absolute form.
	TokenThreshold int64

	// ThresholdFraction expresses the proactive trigger relative to
	// ContextWindow: the trigger fires at ThresholdFraction *
	// ContextWindow input-side tokens. Both must be set; the library
	// does not assume any model's context-window size (E3.5). When
	// TokenThreshold is also set, the smaller effective threshold wins.
	ThresholdFraction float64
	ContextWindow     int64

	// Archive, if non-nil, receives the replaced prefix of each
	// committed compaction before it is discarded.
	Archive ArchiveFunc
}

// enabled reports whether a strategy is configured (S2.18 opt-in).
func (c CompactionConfig) enabled() bool { return c.Strategy != nil }

// proactiveThreshold resolves the configured proactive trigger to an
// absolute token count. Zero means no proactive trigger (S2.19).
func (c CompactionConfig) proactiveThreshold() int64 {
	abs := c.TokenThreshold
	if c.ThresholdFraction > 0 && c.ContextWindow > 0 {
		rel := int64(c.ThresholdFraction * float64(c.ContextWindow))
		if abs == 0 || rel < abs {
			return rel
		}
	}
	return abs
}

// SetCompaction replaces the Agent's compaction configuration in full
// (S2.18). Like SetHooks, it may be called at any time, including
// between Run calls; triggers read the current configuration. The zero
// CompactionConfig disables compaction.
func (a *Agent) SetCompaction(c CompactionConfig) {
	a.compaction = c
}

// Compaction returns the Agent's current compaction configuration (S2.18).
func (a *Agent) Compaction() CompactionConfig {
	return a.compaction
}

// Compact applies the configured strategy to the committed history
// immediately — the manual trigger of S2.19 — and is a no-op when no
// strategy is configured. An explicit Compact is always a committed
// compaction, even when the strategy could otherwise be applied
// transiently: the committed history is mutated and the replaced
// prefix is delivered to the archival callback. A summarizing
// strategy's own LLM call counts toward cumulative usage (S2.20).
func (a *Agent) Compact(ctx context.Context) error {
	if !a.compaction.enabled() {
		return nil
	}
	ctx, span := startSpan(ctx, "agent.compact",
		attribute.String("agent.model", string(a.config.Model)),
	)
	var err error
	defer func() { endSpan(span, err) }()

	_, _, err = a.compactCommitted(ctx, a.conversation.Len(), false)
	return err
}

// compactCommitted runs the configured strategy over the first limit
// messages of the committed history and, when it proposes a cut,
// validates the composed result, delivers the replaced prefix to the
// archival callback, and commits the replacement. Messages at or past
// limit (the in-flight run's appends, when called mid-run) are never
// touched, which keeps the run's rollback anchor meaningful. Returns
// the post-compaction limit and whether a compaction was applied.
// lastRun controls whether a summarizing strategy's usage counts
// toward the last-run component (true mid-run) or cumulative only
// (manual Compact between runs) per S2.20.
func (a *Agent) compactCommitted(ctx context.Context, limit int, lastRun bool) (int, bool, error) {
	res, err := a.runStrategy(ctx, limit, lastRun)
	if err != nil {
		return limit, false, err
	}
	if res.Cut == 0 {
		return limit, false, nil
	}

	msgs := a.conversation.Messages()

	// Compose the new committed prefix in a fresh slice so neither the
	// strategy's Replacement backing array nor the conversation copy is
	// aliased.
	compacted := make([]anthropic.MessageParam, 0, len(res.Replacement)+len(msgs)-res.Cut)
	compacted = append(compacted, res.Replacement...)
	compacted = append(compacted, msgs[res.Cut:limit]...)

	// The post-compaction committed history must satisfy the resumption
	// invariants with no read-side cleanup (S2.18, S2.15).
	if verr := validateHistory(compacted); verr != nil {
		err = fmt.Errorf("compaction: strategy produced invalid history: %w", verr)
		return limit, false, err
	}

	// Archival of the replaced prefix (S2.18): deliver before discard.
	if a.compaction.Archive != nil {
		prefix := make([]anthropic.MessageParam, res.Cut)
		copy(prefix, msgs[:res.Cut])
		a.compaction.Archive(prefix)
	}

	a.conversation.Replace(append(compacted, msgs[limit:]...))
	newLimit := limit - res.Cut + len(res.Replacement)

	a.log.InfoContext(ctx, "compaction applied",
		logArgs(ctx,
			"replaced_messages", res.Cut,
			"replacement_messages", len(res.Replacement),
			"history_len", a.conversation.Len(),
			"archived", a.compaction.Archive != nil,
		)...,
	)
	return newLimit, true, nil
}

// runStrategy invokes the configured strategy over the first limit
// messages of the committed history, bounds-checks its proposal, and
// accumulates any usage the strategy reports (S2.20). The strategy's
// usage is recorded even if the proposal is later rejected — the
// tokens were spent.
func (a *Agent) runStrategy(ctx context.Context, limit int, lastRun bool) (CompactionResult, error) {
	history := a.conversation.Messages()[:limit]
	res, err := a.compaction.Strategy.Compact(ctx, CompactionRequest{
		History:   history,
		Completer: a.completer,
		Model:     a.config.Model,
		MaxTokens: a.config.MaxTokens,
	})
	a.addCallUsage(res.Usage, lastRun)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("compaction: strategy: %w", err)
	}
	if res.Cut < 0 || res.Cut > limit {
		return CompactionResult{}, fmt.Errorf("compaction: strategy cut %d out of range [0, %d]", res.Cut, limit)
	}
	return res, nil
}

// transientMessages computes the transiently-compacted view of the
// conversation for an outgoing request (S2.18): the strategy's
// replacement spliced onto the retained suffix, validated like a
// committed compaction but without mutating committed state and
// without archival — nothing is discarded.
func (a *Agent) transientMessages(ctx context.Context, limit int) ([]anthropic.MessageParam, bool, error) {
	res, err := a.runStrategy(ctx, limit, true)
	if err != nil {
		return nil, false, err
	}
	if res.Cut == 0 {
		return nil, false, nil
	}

	msgs := a.conversation.Messages()
	view := make([]anthropic.MessageParam, 0, len(res.Replacement)+len(msgs)-res.Cut)
	view = append(view, res.Replacement...)
	view = append(view, msgs[res.Cut:limit]...)
	if verr := validateHistory(view); verr != nil {
		return nil, false, fmt.Errorf("compaction: strategy produced invalid history: %w", verr)
	}
	view = append(view, msgs[limit:]...)
	return view, true, nil
}

// applyStrategyForRequest applies the configured strategy over the
// pre-run committed prefix (*startLen) and returns the messages to use
// for the outgoing request. A committed strategy mutates the
// conversation and adjusts *startLen so the run's rollback anchor
// keeps pointing at the same boundary; a transient strategy shapes the
// returned view only. After a committed compaction the conversation-
// size signal is reset so the next iteration doesn't immediately
// re-trigger on stale usage. applied reports whether the strategy
// proposed a change.
func (a *Agent) applyStrategyForRequest(ctx context.Context, startLen *int) ([]anthropic.MessageParam, bool, error) {
	if a.compaction.Strategy.Committed() {
		newStart, applied, err := a.compactCommitted(ctx, *startLen, true)
		if err != nil {
			return nil, false, err
		}
		if applied {
			*startLen = newStart
			a.lastCallInputTokens = 0
		}
		return a.conversation.Messages(), applied, nil
	}
	view, applied, err := a.transientMessages(ctx, *startLen)
	if err != nil {
		return nil, false, err
	}
	if !applied {
		return a.conversation.Messages(), false, nil
	}
	return view, true, nil
}

// isContextOverflow reports whether err is the Anthropic API's
// context-window overflow error — the reactive trigger of S2.19. The
// API reports overflow as an HTTP 400 invalid_request_error whose
// message describes the token excess; there is no dedicated error type,
// so detection matches the documented message forms.
func isContextOverflow(err error) bool {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		return false
	}
	body := strings.ToLower(apiErr.RawJSON())
	return strings.Contains(body, "prompt is too long") ||
		strings.Contains(body, "exceed context limit")
}
