# Change Request: Semantic Versioning and CI-Driven Releases

**Date:** 2026-07-03
**Status:** Accepted

## Ask

Add semantic versioning to the project: branches represent pre-releases,
`main` produces released versions, the version is controlled from a single
place in the source tree per Go convention, CI creates the tags, a release is
cut from a successful merge to `main`, and the process is documented in the
repository.

## Analysis

Go modules take their version from git tags (`vX.Y.Z`), with no
toolchain-owned version file. The prevailing convention for an in-tree source
of truth is a `version.go` constant; here it lives in the library package as
`agent.Version`, holding the exact tag string so constant, tag, and module
version can never disagree.

Release automation is a GitHub Actions workflow triggered on `push`:

- On `main`, a stable `Version` with no matching tag is tagged and published
  as a GitHub Release (after re-running vet/build/test). Merges that do not
  bump the version are release no-ops, so docs and partial-work PRs stay
  cheap.
- On branches, a pre-release-suffixed `Version` (e.g. `v0.2.0-rc.1`) is
  tagged and published as a GitHub pre-release. Stable versions on branches
  are ignored — that is the resting state of every feature branch.
- A CI check on PRs targeting `main` requires any version bump to be stable,
  monotonically increasing, and untagged, front-loading failures to PR time.

Alternatives considered: auto-derived pre-release tags per push (rejected —
tag noise, and the pre-release identity would not live in the source tree)
and manual tagging (rejected — the ask is CI-driven releases).

## PEGS Impact

- **P4 (Tasks and Deliverables):** added P4.5 Versioned Releases.
- **P5 (Required Technology Elements):** added P5.5 GitHub Actions (CI and
  Release Automation), which also records the CI already in use.
- **P7 (Process):** no text change; this file is the first use of the
  `changes/` workflow P7 describes.
- Non-requirements artifacts: `agent/version.go`,
  `.github/workflows/release.yml`, a version check in
  `.github/workflows/ci.yml`, and a "Releases and versioning" section in
  CONTRIBUTING.md.
