# The gate runs the tests under the race detector

**Status:** Accepted

## Context

The provisioning defect behind [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md) appeared only with the race detector on and the test packages running in parallel. Every plain test run passed, including the one the gate ran before review.

The detector changes timing: it is slower and collects more often, which is what starved the in-container scheduler and exposed the defect. The lab's subject is the order in which a control plane reacts, so that regime is the one worth running.

The detector needs cgo and a C compiler on Linux. On macOS the Go toolchain runs it with neither.

A later slice reached the other bound. `go test` gives each package ten minutes by default, and the command's package runs the whole lab per test case: about 413 seconds under the detector for three Scenarios. The fourth Scenario of REQ-SIM-011 takes that past the default, and the lab's Scenarios are what the lab is for.

Nothing in the lab waits without a bound. Every Wait is held to the lab's own waiting time, and the engine's probe, pull and start have timeouts of their own (`system-design.md` § Constants). The package default therefore guards no wait the lab leaves open.

## Options Considered

1. **A hand-run check** — the detector recommended in the testing brief, run when someone remembers.
2. **A separate gate check, skipped where cgo is unavailable** — required on a machine that can run it.
3. **The gate's test check runs the detector** — one test run, and no machine passes the gate without it.
4. **The gate demands cgo and a C compiler before running** — a precondition the Makefile checks itself, by operating system or on every machine.

How long the gate gives a package:

5. **The default kept, the command's test cases restructured** — one run of the lab shared by the cases reading it, which also cuts the suite's time.
6. **A stated timeout on the gate's run** — one flag, above what every Scenario the lab ships can take.
7. **A timeout stated per test case** — a bound in each case that runs the lab, beside the bounds the lab already holds.

## Decision

Option 3. The gate runs the tests once, under the race detector, on every machine. The gate asserts no toolchain prerequisite of its own. The Go toolchain refuses where the detector needs cgo and a C compiler it cannot find. That refusal fails the gate, and the target names how to install a compiler.

Option 1 is rejected because the defect it would have caught shipped to review instead.

Option 2 is rejected because the machine that skips the check is the machine whose result is then trusted.

Option 4 is rejected because its precondition is a second rule about where the detector needs a compiler. An operating-system branch guesses, and demanding a compiler on every machine fails a macOS run that the detector supports.

Option 6. The gate's test run states a timeout of its own, several times the longest package measured. A Scenario added to the lab then does not fail the gate on the clock. The Makefile owns the value.

Option 5 is not rejected: it is the next step when the suite nears the stated timeout again. It changes the command's tests rather than the gate, and taking it here would fold a test refactor into the slice that added a Scenario.

Option 7 is rejected because the lab already bounds every wait it makes. A second bound per case would state the same limit in as many places as there are cases.

## Consequences

Every gate run pays the detector's slower timing. A run's cost stays dominated by the container starts a Scenario pays.

A failure under the detector is a defect to fix. Dropping the check is not a way to make it pass.

A contributor on Linux installs a C compiler (`CLAUDE.md` § Toolchain). macOS needs none.

A stuck run now ends at a wait of the lab's own, naming what it waited for, rather than at the toolchain's deadline naming the package. The stated timeout is raised by a reviewed change where the lab outgrows it, and the gate's wall time stays the Scenarios' own cost.

## Implementation

**Requirements:** REQ-SIM-004, REQ-SIM-001, REQ-SIM-011

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: Provisioning ends when the scheduler has placed a Pod on every Node](2026-10-04-scheduler-observable-provisioning.md)
