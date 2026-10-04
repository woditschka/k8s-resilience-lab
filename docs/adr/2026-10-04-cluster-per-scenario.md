# Each Scenario provisions its own Simulated Cluster

**Status:** Accepted

## Context

A Scenario's asserted Placement is only trustworthy when nothing from another Scenario can reach it. A leftover Node, a Pod still terminating, or a Scenario that failed halfway is enough to change the next assertion.

The lab also runs one named Scenario on its own (REQ-SIM-003). That invocation has to produce the same outcome the full run reports for it.

Two lab invocations share one machine — a developer's run beside a build job — and neither may disturb the other.

## Options Considered

1. **One Simulated Cluster per run, cleaned between Scenarios** — one container start per run, and cleanliness that depends on the cleanup code being complete.
2. **One long-lived Simulated Cluster reused across runs** — the fastest, and every outcome depends on what ran before.
3. **One Simulated Cluster per Scenario** — a container start per Scenario, and isolation that depends on nothing.

## Decision

Option 3. A Scenario runs on a Simulated Cluster provisioned for it alone, removed when the Scenario ends, including when it fails or panics. The runner holds the handle and closes it, so a Scenario's own code carries no teardown.

Scenarios run one after another. No package holds mutable process-wide state: the resolved engine endpoint is an immutable value passed into each provisioning call, so running Scenarios concurrently stays a change to the runner alone.

Each cluster container's name carries the Scenario it serves and a suffix unique to the invocation, and the engine chooses each published port. A run removes only the container its own handle names, per [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md).

## Consequences

A run of N Scenarios pays N container starts, sequentially. The lab has one Scenario, so the cost is not yet visible, and isolation is worth it at the sizes a lab runs.

One named Scenario reports what the full run reports for it, because neither has a predecessor.

Two invocations on one machine cannot collide. Their container names differ, their ports differ, and neither can name the other's container.

At most one Simulated Cluster exists at a time, which keeps a run's footprint one container whatever a Scenario declares.

## Implementation

**Requirements:** REQ-SIM-001

## References

- [system-design.md § State Machine](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
