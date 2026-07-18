# go-agents

A Go agent-harness library. Public surface is the single
`github.com/rfbigelow/go-agents/agent` package.

## Before every commit

Run all four CI checks — they are the source of truth (see
CONTRIBUTING.md), and CI will reject a push that fails any of them:

```sh
gofmt -l .      # must print nothing; fix with gofmt -w .
go vet ./...
go build ./...
go test ./...
```

`gofmt -l .` is the one that is easy to forget: hand-aligned comment
columns and trailing blank lines both fail it.

## Workflow conventions

- **Spec first (PEGS).** Requirements in `requirements/` are the source
  of truth; behavior changes keep them in sync. Requirement IDs (S2.x,
  S6.x, …) go in commit messages and PR bodies. Decisions made during
  implementation that resolve spec latitude are recorded in `changes/`
  (see existing files there for the format) with matching spec edits in
  the same PR.
- **Everything merges via PR** — `main` is protected, including for the
  maintainer. `gh pr merge --auto --squash` once CI is green.
- **Versioning.** `agent/version.go` is the single source of truth; the
  `const Version = ` line must stay single-line (CI greps for it).
  Bump only in a release-worthy PR; tags are cut by CI, never by hand.
- **Verification.** For library changes, also verify through the public
  package boundary using the `verify` skill (standalone consumer module),
  not just `go test`.
