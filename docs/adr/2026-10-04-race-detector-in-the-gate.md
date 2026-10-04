# The gate runs the tests under the race detector

**Status:** Accepted

## Context

The provisioning defect behind [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md) appeared only with the race detector on and the test packages running in parallel. Every plain test run passed, including the one the gate ran before review.

The detector changes timing: it is slower and collects more often, which is what starved the in-container scheduler and exposed the defect. The lab's subject is the order in which a control plane reacts, so that regime is the one worth running.

The detector needs cgo and a C compiler on Linux. On macOS the Go toolchain runs it with neither.

## Options Considered

1. **A hand-run check** — the detector recommended in the testing brief, run when someone remembers.
2. **A separate gate check, skipped where cgo is unavailable** — required on a machine that can run it.
3. **The gate's test check runs the detector** — one test run, and no machine passes the gate without it.
4. **The gate demands cgo and a C compiler before running** — a precondition the Makefile checks itself, by operating system or on every machine.

## Decision

Option 3. The gate runs the tests once, under the race detector, on every machine. The gate asserts no toolchain prerequisite of its own. The Go toolchain refuses where the detector needs cgo and a C compiler it cannot find. That refusal fails the gate, and the target names how to install a compiler.

Option 1 is rejected because the defect it would have caught shipped to review instead.

Option 2 is rejected because the machine that skips the check is the machine whose result is then trusted.

Option 4 is rejected because its precondition is a second rule about where the detector needs a compiler. An operating-system branch guesses, and demanding a compiler on every machine fails a macOS run that the detector supports.

## Consequences

Every gate run pays the detector's slower timing. A run's cost stays dominated by the container starts a Scenario pays.

A failure under the detector is a defect to fix. Dropping the check is not a way to make it pass.

A contributor on Linux installs a C compiler (`CLAUDE.md` § Toolchain). macOS needs none.

## Implementation

**Requirements:** REQ-SIM-004, REQ-SIM-001

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
