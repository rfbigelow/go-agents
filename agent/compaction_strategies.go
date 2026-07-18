package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// defaultSummaryMaxTokens caps the hybrid strategy's summary call when
// HybridSummarizationStrategy.SummaryMaxTokens is zero.
const defaultSummaryMaxTokens = 1024

// defaultSummaryPrompt instructs the summarization call when
// HybridSummarizationStrategy.Prompt is empty.
const defaultSummaryPrompt = "You are summarizing the earlier portion of a " +
	"conversation so it can be replaced with a compact summary. Capture the " +
	"user's goals, key facts, decisions, tool actions and their results, and " +
	"any unresolved threads. Be concise but complete: the rest of the " +
	"conversation must remain coherent when it follows only this summary."

// SlidingWindowStrategy is the library-provided truncation strategy
// (S2.21): it drops the oldest turns, cutting only on fresh-turn
// boundaries, and keeps a fixed recent window. It is deterministic and
// makes no LLM call, so it may run transiently (the default); set
// Commit to mutate committed history instead, which bounds memory
// (E3.5) and delivers the dropped prefix to the archival callback.
type SlidingWindowStrategy struct {
	// KeepTurns is the number of most-recent fresh turns retained.
	// Zero or negative disables the strategy (it never proposes a cut).
	KeepTurns int

	// AdvanceChunk is the granularity, in turns, at which the window's
	// leading edge advances. Larger chunks keep the request prefix
	// stable across more consecutive calls, compacting infrequently
	// and substantially rather than trimming a little each turn
	// (S2.18 cache guidance, S2.17). Zero means 1.
	AdvanceChunk int

	// Commit, when true, makes the strategy a committed one: the
	// dropped prefix is removed from committed history (and archived,
	// S2.18) instead of being elided from outgoing requests only.
	Commit bool
}

// Compact proposes dropping the oldest turns beyond the retained
// window, rounded down to AdvanceChunk granularity.
func (s *SlidingWindowStrategy) Compact(_ context.Context, req CompactionRequest) (CompactionResult, error) {
	starts := turnStarts(req.History)
	n := len(starts)
	if s.KeepTurns <= 0 || n <= s.KeepTurns {
		return CompactionResult{}, nil
	}
	chunk := s.AdvanceChunk
	if chunk < 1 {
		chunk = 1
	}
	drop := (n - s.KeepTurns) / chunk * chunk
	if drop == 0 {
		return CompactionResult{}, nil
	}
	return CompactionResult{Cut: starts[drop]}, nil
}

// Committed reports the configured commitment mode (S2.18): sliding-
// window truncation is deterministic, so transient application is
// permitted.
func (s *SlidingWindowStrategy) Committed() bool { return s.Commit }

// HybridSummarizationStrategy is the library-provided summarizing
// strategy (S2.21, the recommended default): it replaces an older
// prefix of the history with a single generated summary message and
// retains a recent verbatim window of turns. It invokes the Agent's
// Completer to produce the summary, so it is non-deterministic and is
// always committed (S2.18).
type HybridSummarizationStrategy struct {
	// KeepTurns is the number of most-recent fresh turns retained
	// verbatim after the summary. Zero or negative disables the
	// strategy (it never proposes a cut).
	KeepTurns int

	// SummaryMaxTokens caps the summary generation call. Zero means
	// defaultSummaryMaxTokens.
	SummaryMaxTokens int64

	// Prompt overrides the default summarization instruction.
	Prompt string
}

// Committed is always true: LLM summarization is non-deterministic and
// expensive, so its output must be computed once and committed (S2.18).
func (s *HybridSummarizationStrategy) Committed() bool { return true }

// Compact summarizes everything before the retained window and
// replaces it with a single user-role summary message. The cut lands
// on the assistant message that closes the turn preceding the retained
// window, so the compacted history alternates correctly: the summary
// (user) is followed by that assistant message, then the retained
// fresh turns. The resumption invariants guarantee that assistant
// message carries no unresolved tool_use, making the boundary safe.
func (s *HybridSummarizationStrategy) Compact(ctx context.Context, req CompactionRequest) (CompactionResult, error) {
	starts := turnStarts(req.History)
	n := len(starts)
	if s.KeepTurns <= 0 || n <= s.KeepTurns {
		return CompactionResult{}, nil
	}
	// Index of the first retained fresh turn; the message before it is
	// the assistant message that will follow the summary.
	boundary := starts[n-s.KeepTurns]
	cut := boundary - 1
	if cut <= 0 {
		return CompactionResult{}, nil
	}

	summary, usage, err := s.summarize(ctx, req, req.History[:cut])
	if err != nil {
		return CompactionResult{Usage: usage}, err
	}

	replacement := anthropic.NewUserMessage(anthropic.NewTextBlock(
		"[Summary of the earlier conversation, which has been compacted]\n\n" + summary,
	))
	return CompactionResult{
		Cut:         cut,
		Replacement: []anthropic.MessageParam{replacement},
		Usage:       usage,
	}, nil
}

// summarize generates the summary via the Agent's Completer (S2.14).
// The prefix is flattened into a plain-text transcript inside a single
// user message, so the summary call is always protocol-valid
// regardless of where the cut landed relative to tool-use turns.
func (s *HybridSummarizationStrategy) summarize(ctx context.Context, req CompactionRequest, prefix []anthropic.MessageParam) (string, anthropic.Usage, error) {
	if req.Completer == nil {
		return "", anthropic.Usage{}, fmt.Errorf("hybrid summarization requires a Completer")
	}
	prompt := s.Prompt
	if prompt == "" {
		prompt = defaultSummaryPrompt
	}
	maxTokens := s.SummaryMaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultSummaryMaxTokens
	}

	summaryReq := CompletionRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: prompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(
				"Summarize the following conversation:\n\n" + renderTranscript(prefix),
			)),
		},
	}

	stream, err := req.Completer.Complete(ctx, summaryReq)
	if err != nil {
		return "", anthropic.Usage{}, fmt.Errorf("summary call: %w", err)
	}
	defer stream.Close()
	for stream.Next() {
		// Drain; the summary is read from the accumulated message.
	}
	if err := stream.Err(); err != nil {
		return "", anthropic.Usage{}, fmt.Errorf("summary stream: %w", err)
	}
	msg := stream.Message()

	var sb strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	if sb.Len() == 0 {
		return "", msg.Usage, fmt.Errorf("summary call returned no text")
	}
	return sb.String(), msg.Usage, nil
}

// turnStarts returns the indices of messages that begin a fresh turn:
// user messages carrying no tool_result blocks. Fresh-turn boundaries
// are always safe cut points — a tool_result-carrying user message is
// never separated from the assistant tool_use message preceding it,
// and retained turns keep their thinking blocks verbatim because
// retained messages are untouched (S2.18).
func turnStarts(msgs []anthropic.MessageParam) []int {
	var starts []int
	for i, m := range msgs {
		if m.Role != anthropic.MessageParamRoleUser {
			continue
		}
		fresh := true
		for _, block := range m.Content {
			if block.OfToolResult != nil {
				fresh = false
				break
			}
		}
		if fresh {
			starts = append(starts, i)
		}
	}
	return starts
}

// renderTranscript flattens messages into a plain-text transcript for
// the summary call. Text and tool activity are included; thinking
// blocks are omitted — they are the model's internal reasoning, not
// conversation content.
func renderTranscript(msgs []anthropic.MessageParam) string {
	var sb strings.Builder
	for _, m := range msgs {
		role := "User"
		if m.Role == anthropic.MessageParamRoleAssistant {
			role = "Assistant"
		}
		for _, block := range m.Content {
			switch {
			case block.OfText != nil:
				fmt.Fprintf(&sb, "%s: %s\n", role, block.OfText.Text)
			case block.OfToolUse != nil:
				fmt.Fprintf(&sb, "%s: [called tool %s with %s]\n", role, block.OfToolUse.Name, block.OfToolUse.Input)
			case block.OfToolResult != nil:
				fmt.Fprintf(&sb, "%s: [tool result: %s]\n", role, renderToolResult(block.OfToolResult))
			}
		}
	}
	return sb.String()
}

// renderToolResult extracts the text of a tool_result block's content
// for the transcript.
func renderToolResult(tr *anthropic.ToolResultBlockParam) string {
	var sb strings.Builder
	for _, c := range tr.Content {
		if c.OfText != nil {
			sb.WriteString(c.OfText.Text)
		}
	}
	return sb.String()
}
