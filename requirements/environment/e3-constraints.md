# E3: Constraints

## Technical

### E3.1: Go 1.25+

**Source:** Developer choice, based on language feature availability.
**Description:** The library requires Go 1.25 or later.
**Impact:** The library may use any Go language features and standard library
APIs available in Go 1.25.

### E3.2: Minimal External Dependencies

**Source:** Developer preference.
**Description:** The library should minimize external dependencies beyond the
Go standard library and the Anthropic Go SDK. The OpenTelemetry Trace API
(E2.3) is an accepted dependency, justified by its role as the industry
standard for distributed tracing, its lightweight nature (no-op without a
consumer-configured SDK), and the impracticality of achieving equivalent
functionality through stdlib alone. Any further dependencies require explicit
justification.
**Impact:** Prefer stdlib solutions. Avoid dependency on large frameworks or
libraries that pull in transitive dependency trees.

### E3.3: Platform Agnosticism

**Source:** Developer preference. Deployment target is GCP, but the library
should not be coupled to it.
**Description:** The library must not depend on any specific cloud platform,
operating system, or deployment environment.
**Impact:** Platform-specific concerns (e.g., credential management, logging
backends, deployment configuration) are the responsibility of the consuming
application, not the library.

### E3.5: Consumer Resource Control

**Source:** Go library conventions.
**Description:** The library must not take exclusive ownership of resources in
ways that prevent the consuming application from controlling their lifecycle.
Consumers should be able to influence memory-significant decisions such as
conversation history bounds and compaction strategy.
**Impact:** Features that manage long-lived resources (e.g., conversation state)
must provide mechanisms for the consumer to control resource usage. The library
provides sensible defaults but does not impose them.

## Legal

### E3.4: MIT License Compatibility

**Source:** Developer preference.
**Description:** The library will be MIT-licensed. All dependencies must have
licenses compatible with MIT (e.g., MIT, BSD, Apache 2.0).
**Impact:** Dependencies with copyleft licenses (e.g., GPL) are not
acceptable.
