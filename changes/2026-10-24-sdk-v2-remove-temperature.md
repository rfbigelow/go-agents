# Change Request: Adopt Anthropic Go SDK v2; Remove Temperature

**Date:** 2026-10-24
**Status:** Accepted

## Ask

Update the library from Anthropic Go SDK v1.56.0 to the latest release
(v2.3.0) and record the consequences for the spec.

## Analysis

SDK v2 is a new Go module (`github.com/anthropics/anthropic-sdk-go/v2`)
and targets Messages API version `2026-10-21`. v1 was last released as
v1.84.0 on 2026-10-20, with no maintenance branch or support statement
since, so staying on v1 would forgo new models and fixes. Three v2
changes touch this library:

1. **Sampling parameters are removed.** `temperature`, `top_p`, and
   `top_k` are gone from the SDK, and the API returns 400 if they are
   sent. The Agent's and Completer's `temperature` parameter (S2.1, and
   the Completer command-query table) cannot be honored. Options were to
   remove it or keep it as a silent no-op; a silently ignored setting
   misleads callers, so it is removed. Callers who relied on a
   non-default temperature lose that control on models that accepted it.
2. **`MessageParam` is a union** (`OfUser`, `OfAssistant`, `OfSystem`)
   in place of a struct with `Role` and `Content`. This is a mechanical
   change to the library's internals; the conversation read operation and
   resumption constructor (S2.15) keep their contract, but consumers
   must import the v2 package to name the message types.
3. **Go 1.25+** is required, which E3.1 (Go 1.26+) already satisfies.

The SDK's dependency on a newer OpenTelemetry release raised the
`go.opentelemetry.io/otel` requirements from v1.24.0 to v1.33.0 as a side
effect; E3.2 (minimal dependencies) is unaffected.

## PEGS Impact

- **S2.1 (Agent Creation and Configuration):** the `temperature`
  configuration parameter is removed.
- **Completer command-query table:** the `temperature` parameter is
  removed.
- **E2.2 / E2.1 (Environment components):** record SDK major version 2,
  API version `2026-10-21`, and the removal of sampling parameters.
- **E4.3 (SDK stability):** notes the observed v2 break.
- **Release:** a breaking change to the public API (`Config.Temperature`,
  `SubAgentDef.Temperature`, `CompletionRequest.Temperature` removed;
  SDK types now come from the v2 module), so `Version` moves to
  `v0.5.0`.
- No other requirements change.
