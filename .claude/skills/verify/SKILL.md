---
name: verify
description: How to verify go-agents library changes end-to-end through the public package boundary.
---

# Verifying go-agents changes

This is a library; its surface is the `github.com/rfbigelow/go-agents/agent`
package boundary. Verify by building a small standalone consumer, not by
re-running `go test`.

## Recipe

1. Create a scratch dir outside the repo with its own module:

   ```bash
   go mod init scratch-verify
   go mod edit -replace github.com/rfbigelow/go-agents=/Users/rfbigelow/repos/go-agents
   go mod tidy
   ```

2. Without an `ANTHROPIC_API_KEY`, implement the public `agent.Completer`
   interface in the consumer and return streams from the exported
   `agent.NewTestEventStream(texts, msg)`. Gotcha: the `anthropic.Message`
   you hand it must be JSON round-tripped (`json.Marshal` then `Unmarshal`
   into itself) or `Message.ToParam()` silently drops content when the
   agent appends the assistant turn.

3. With a key present, `agent.NewAnthropicCompleter(anthropic.NewClient())`
   against the live API also works; the `examples/` programs show the
   pattern (`go run ./examples/chat/` etc.).

4. Drive flows that cross real boundaries: persist `agent.Conversation()`
   to JSON in one process, reload and resume via `agent.NewAgentWithHistory`
   in a second process; tamper the JSON with `jq` to hit validation errors.

## Notes

- The agent logs INFO lines to stderr by default (`Config.Logger` unset);
  filter with `grep -v INFO` when capturing output.
- Prompt caching is on by default and adds `cache_control` to outgoing
  request copies; set `Config.DisablePromptCaching: true` when comparing
  request messages byte-for-byte against seeded history.
