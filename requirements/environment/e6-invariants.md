# E6: Invariants

### E6.1: Application Controls Execution Flow

**Must always be true:** The consuming application initiates all interactions
with the library and receives control back when each interaction completes. The
library must not take over the main loop, spawn unmanaged background work, or
block indefinitely without a mechanism for the application to cancel.
**Rationale:** The library is a component within a larger application. If it
seizes control of execution flow, it prevents the application from managing its
own lifecycle — handling signals, enforcing timeouts, or coordinating with other
subsystems.

### E6.2: Anthropic API Protocol Compliance

**Must always be true:** All messages sent to the Anthropic Messages API (E2.1)
conform to the API's protocol: valid message sequences, role alternation rules,
and content format requirements.
**Rationale:** Violating the API contract produces errors or undefined behavior
from the LLM provider. The library mediates all API communication (E5.2), so
protocol compliance is its responsibility.

### E6.3: Consumer Resource Ownership

**Must always be true:** The consuming application can manage the lifecycle of
all resources it provides to or obtains from the library — including canceling
in-flight requests, bounding memory usage, and shutting down cleanly.
**Rationale:** Extends E3.5 (Consumer Resource Control) as an invariant: the
library must never hold resources hostage or prevent the application from
reclaiming them. Violation would make the library unsuitable for production
use in long-running applications.
