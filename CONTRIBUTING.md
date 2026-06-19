# Contributing

Thanks for working on go-agents. This document describes the development
workflow.

## Prerequisites

- Go 1.26 or later (the module's `go` directive; CI pins to it via
  `go-version-file: go.mod`).

## Local checks

CI runs four checks on every push and pull request. Run the same commands
locally before opening a PR — they are the source of truth, and there is
deliberately no Makefile or task runner wrapping them (see requirement P5.3):

```sh
gofmt -l .      # lists unformatted files; output should be empty
go vet ./...
go build ./...
go test ./...
```

If `gofmt -l .` reports any files, format them in place:

```sh
gofmt -w .
```

## Pull request workflow

`main` is a protected branch: direct pushes are rejected (including for
admins), so every change goes through a pull request.

```sh
git checkout -b my-change
# ...make changes, commit...
git push -u origin my-change
gh pr create --fill
gh pr merge --auto --squash   # merges automatically once CI is green
```

### Branch protection rules on `main`

- A pull request is required for every change.
- The CI `build` check must pass before merging, and the branch must be up to
  date with `main` first.
- No approving review is required — you may merge your own PR once CI is green.
- Force-pushes and branch deletion are blocked.

## Requirements

This project follows the PEGS method; see
[requirements/](requirements/README.md) for the full specification. Changes to
behavior should keep the requirements in sync.
