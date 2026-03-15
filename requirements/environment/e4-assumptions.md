# E4: Assumptions

### E4.1: Anthropic Is the Only LLM Provider

**We assume that:** The library only needs to support Anthropic's API. No
multi-provider abstraction layer is needed.
**This matters because:** It allows the library to use Anthropic-specific
features (e.g., tool-use protocol, extended thinking) directly rather than
abstracting to a lowest-common-denominator interface.
**If wrong:** If additional providers are needed later, a provider abstraction
layer would need to be introduced. This could be a significant refactor
depending on how tightly the library is coupled to Anthropic's SDK types.

### E4.2: Single Developer

**We assume that:** The library is developed and maintained by a single
person. There are no team coordination concerns.
**This matters because:** It simplifies process decisions (branching strategy,
review workflow, release process).
**If wrong:** If the repo is made public and attracts contributors,
contribution guidelines, CI checks, and a review process would be needed.

### E4.3: Anthropic Go SDK Stability

**We assume that:** The Anthropic Go SDK provides a reasonably stable API
surface and follows semantic versioning.
**This matters because:** The library depends on SDK types and functions
throughout its API. Frequent breaking changes in the SDK would require
frequent library updates.
**If wrong:** May need to introduce an anti-corruption layer or pin to
specific SDK versions.
