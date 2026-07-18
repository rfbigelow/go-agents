# Change Request: Compaction Implementation Clarifications

**Date:** 2026-07-18
**Status:** Accepted

## Ask

While implementing S2.18–S2.21 (Context Compaction, M10), two points the
spec leaves open needed a concrete choice. Record the choices and align
the spec wording:

1. Which usage figure the proactive trigger compares against its
   threshold.
2. Whether a summarizing strategy's call triggered by manual `compact`
   (outside any run) counts toward the last-run usage component.

## Analysis

S2.19 says the proactive trigger fires "once the conversation's token
usage (S2.20) crosses that threshold". TOKEN_USAGE carries cumulative
totals and last-run usage, but neither is the right conversation-size
signal: cumulative input tokens grow superlinearly (each call re-sends
the history), and last-run totals include output tokens and any
summarization call. The measure that tracks proximity to the context
window is the input side of the most recent main-loop LLM call — input
plus cache-creation plus cache-read tokens — since that is precisely
what the conversation cost to send last time. The implementation
compares that figure against the threshold, excludes a summarizing
strategy's own call from it (its input is the prefix being summarized,
not the conversation), and resets it after a committed compaction so
the next iteration does not re-trigger on stale usage.

The ADT table's usage-column footnote (added in PR #13) defines `+` as
"adding to the cumulative totals and recording the increment as the
last-run component", written with the `run` row in mind. Applying it
literally to the `compact` row would make a manual compact's summary
call replace the last-run component, breaking LastRun's meaning of
"usage attributable to the most recent run" — a manual compact is not
a run. The implementation counts a manual compact's summarization
usage toward Cumulative only; a compaction triggered mid-run counts
toward both, since it is part of that run (per the `run` row's
"including any summarization call").

## PEGS Impact

- **S2.19 (Compaction Triggers):** the proactive rule now names the
  signal — the input-side token count of the most recent LLM call as
  reported through S2.20.
- **S2.20 (Token Usage Reporting):** the summarization-call rule now
  states that a summarizing call triggered by manual `compact` counts
  toward cumulative usage only, while one triggered mid-run also counts
  toward that run's usage.
- No other requirements change.
