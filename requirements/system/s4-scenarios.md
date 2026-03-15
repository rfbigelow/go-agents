# S4: Detailed Usage Scenarios

<!-- Detailed scenarios including special cases, error handling, and
     system-aware details. These extend the high-level scenarios in g5.

     Where g5 describes the "happy path" in user terms, s4 covers:
     - Alternate flows (valid but less common paths)
     - Error cases (what happens when things go wrong)
     - Edge cases (boundary conditions)
     - System behavior details (referencing s1 components and s2 functions)

     Each scenario should have a unique identifier (S4.1, S4.2, ...) and
     should reference the g5 scenario it elaborates, if applicable.

     These scenarios serve as the bridge between requirements and testing:
     each one implies at least one test case (see s6). -->

## S4.1: [Scenario Name]

**Elaborates:** <!-- Reference to g5 scenario, e.g., "G5.1" -->
**Preconditions:** <!-- What must be true before this scenario starts? -->
**Actor:** <!-- Who or what initiates this? -->

**Main flow:**

1. <!-- Step-by-step, including system behavior.
      Unlike g5, this can reference system components and functions.
      e.g., "The API gateway (s1.2) validates the request against
      rate limits (e3.4)." -->

**Alternate flows:**

- **[Condition]:** <!-- What happens differently and when? -->

**Error cases:**

- **[Error condition]:** <!-- How does the system respond? What does
                             the user see? -->

## S4.2: [Scenario Name]

<!-- Continue for each detailed scenario. -->
