package agent

// Version is the semantic version of the go-agents module, in exactly the
// form used for the corresponding git tag and Go module version (a leading
// "v", e.g. "v1.2.3").
//
// It is the single source of truth for releases: when a commit on main
// carries a Version with no matching git tag, CI tags that commit and
// publishes a GitHub Release. A Version with a pre-release suffix (e.g.
// "v0.2.0-rc.1") pushed on a non-main branch produces a GitHub pre-release
// instead, and cannot be merged to main. See the "Releases and versioning"
// section of CONTRIBUTING.md.
//
// The single-line form of this declaration is load-bearing: the CI and
// release workflows locate the value by matching `^const Version = ` and
// assert the match is unique. Do not fold it into a const block.
const Version = "v0.4.0"
