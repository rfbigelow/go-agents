package agent

import "github.com/anthropics/anthropic-sdk-go"

// UsageTotals is one component of TokenUsage: token counts summed over
// one or more API responses, broken down into input, output, and cache
// (creation/read) tokens as reported by the Anthropic API (S2.20).
type UsageTotals struct {
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
}

// add accumulates one API response's reported usage into the totals.
func (u *UsageTotals) add(usage anthropic.Usage) {
	u.InputTokens += usage.InputTokens
	u.OutputTokens += usage.OutputTokens
	u.CacheCreationInputTokens += usage.CacheCreationInputTokens
	u.CacheReadInputTokens += usage.CacheReadInputTokens
}

// TokenUsage is the record returned by the Agent's Usage query (S2.20):
// the conversation's cumulative totals plus the usage attributable to
// the most recent Run. Each Run adds its reported usage to Cumulative
// and replaces LastRun. Cumulative includes every Completer call the
// Agent makes on the conversation's behalf, including a summarizing
// compaction strategy's own call (S2.21); reported values reflect what
// the API returned, including the effect of prompt caching (S2.17).
type TokenUsage struct {
	Cumulative UsageTotals
	LastRun    UsageTotals
}
