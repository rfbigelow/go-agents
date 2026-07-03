# Contributing

This is a personal project built to practice the
[PEGS](requirements/README.md) requirements method. The repository is public for
reference and learning, but **it is not open to outside contributions at this
time** — unsolicited pull requests and issues may be closed without review.

The rest of this document records the development workflow for the maintainer.

## Prerequisites

- Go 1.26 or later (the module's `go` directive; CI pins to it via
  `go-version-file: go.mod`).

## Local checks

CI runs five checks on every push and pull request. Four of them are plain Go
commands — run the same commands locally before pushing; they are the source
of truth, and there is deliberately no Makefile or task runner wrapping them
(see requirement P5.3). The fifth (a version check, see "Releases and
versioning" below) is CI-only and needs nothing beyond keeping
`agent/version.go` sane:

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
admins), so every change — including the maintainer's own — goes through a
pull request.

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
- No approving review is required — the maintainer may merge their own PR once
  CI is green.
- Force-pushes and branch deletion are blocked.

## Releases and versioning

The module follows [semantic versioning](https://semver.org). The single
source of truth is the `Version` constant in `agent/version.go`, whose value
matches the git tag and Go module version exactly (leading `v`, e.g.
`v0.1.0`). Releases are cut by CI (`.github/workflows/release.yml`) — never
create version tags by hand.

### Cutting a stable release

1. In the PR that completes the release-worthy change (or a dedicated PR),
   bump `Version` to the next stable semver, e.g. `v0.2.0`.
2. Merge as usual (`gh pr merge --auto --squash`). CI validates on the PR
   that the new version is stable, greater than `main`'s, and not already
   tagged.
3. On the push to `main`, the release workflow re-runs vet/build/test, tags
   the squash commit `v0.2.0`, and publishes a GitHub Release with generated
   notes.

PRs that do not bump `Version` (docs, refactors, partial work) merge as
usual; the release workflow sees the tag already exists and exits as a no-op.

### Cutting a pre-release

Set `Version` to a pre-release version (e.g. `v0.2.0-rc.1`) and push the
branch. The release workflow tags that commit and publishes a GitHub
*pre-release*. Each iteration needs a new pre-release number (`-rc.2`, ...).
Before merging to `main`, set `Version` to the final stable version — CI
blocks merging a pre-release version into `main`.

### Rules and caveats

- Tags served by the Go module proxy are cached forever. Never delete and
  re-create a tag; ship a new patch version instead.
- Tags are created with the workflow's `GITHUB_TOKEN`, so tag creation does
  not trigger other workflows. The GitHub Release is created in the same job,
  so nothing else needs to fire. (A future `on: release` workflow would need
  a PAT or GitHub App token.)
- A pre-release tag on an abandoned branch keeps its commit alive. If the
  module proxy never served it, clean up with
  `gh release delete vX.Y.Z-rc.N --cleanup-tag`; otherwise just abandon that
  pre-release number.
- If two PRs bump to the same version, the first to merge gets the tag; the
  second merges as a no-op and its changes ship with the next bump.

## Requirements

This project follows the PEGS method; see
[requirements/](requirements/README.md) for the full specification. Changes to
behavior should keep the requirements in sync.
