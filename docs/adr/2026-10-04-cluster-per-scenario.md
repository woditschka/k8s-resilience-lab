# Each Scenario provisions its own Simulated Cluster

**Status:** Accepted, amended 2026-10-09: Scenarios run at once (REQ-SIM-014)

## Context

A Scenario's asserted Placement is only trustworthy when nothing from another Scenario can reach it. A leftover Node, a Pod still terminating, or a Scenario that failed halfway is enough to change the next assertion.

The lab also runs one named Scenario on its own (REQ-SIM-003). That invocation has to produce the same outcome the full run reports for it.

Two lab invocations share one machine — a developer's run beside a build job — and neither may disturb the other.

The amendment's context: each Scenario spends 7 to 17 seconds inside its cluster, yet about a minute of provisioning and removal surrounds each one. Run one after another, the shipped Scenarios cost a run several minutes, and the command tests about 15 minutes (REQ-SIM-014).

## Options Considered

1. **One Simulated Cluster per run, cleaned between Scenarios** — one container start per run, and cleanliness that depends on the cleanup code being complete.
2. **One long-lived Simulated Cluster reused across runs** — the fastest, and every outcome depends on what ran before.
3. **One Simulated Cluster per Scenario** — a container start per Scenario, and isolation that depends on nothing.

## Decision

Option 3. A Scenario runs on a Simulated Cluster provisioned for it alone, removed when the Scenario ends, including when it fails or panics. The runner holds the handle and closes it, so a Scenario's own code carries no teardown.

Scenarios run at once, each in a goroutine of its own, since the amendment of 2026-10-09. No package holds mutable process-wide state: the resolved engine endpoint is an immutable value passed into each provisioning call, so the change is the runner's alone. The runner derives every Scenario's run name before it provisions any cluster, so a name it cannot derive still stops the run before a cluster exists. It then obtains the Kubernetes version's cluster image once, before any Scenario starts. A version the lab cannot obtain stops the run once, naming it, and no Scenario starts (REQ-SIM-001 edge case 4). A failure confined to one Scenario's cluster stays that Scenario's own. The runner joins every goroutine before returning and hands back the Outcomes in declared order, which is the order the report prints them in.

One Scenario's failure is its own. A cluster the engine cannot start, a Step left undone, a Finding or a panic ends that Scenario alone. The runner recovers a panic into that Scenario's own error, the lab's failure rather than a Step's Cause. A panic left to cross a goroutine would end the process before the other Scenarios' removals run, leaking their containers.

The run bounds nothing: it holds as many clusters as Scenarios are shipped. A machine short of room for them is an Open Question of the PRD. The lab's own tests bound how many run at once per package through the gate's `-parallel` flag.

Each cluster container's name carries the Scenario it serves and a random suffix drawn per provisioning, and the engine chooses each published port on the loopback address. Two clusters of one run, of two concurrent tests, or of two invocations therefore never share a name or a port. A run removes only the container its own handle names, per [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md).

## Consequences

A run of N Scenarios pays N container starts at once, so it lasts about as long as its slowest Scenario. Isolation is worth that container count at the sizes a lab runs.

One named Scenario reports what the full run reports for it, because neither has a predecessor.

Two invocations on one machine cannot collide. Their container names differ, their ports differ, and neither can name the other's container.

A run's footprint is one container per Scenario shipped, which grows with the Scenarios. Before the amendment it was one container at a time. Should a machine lack the room, the PRD's Open Question holds the remedy: a run one at a time.

The gate's footprint is each cluster-backed package's parallel tests times the clusters each holds, across the packages `go test` runs at once. Command tests judging a run of every Scenario share one run, which keeps that product small.

A missing image is pulled once per run, before the fan-out, within the pull timeout. The Scenarios of a run never pull at once, and a run's first Scenario starts only after that pull.

## Implementation

**Requirements:** REQ-SIM-001, REQ-SIM-014

**Files:** `internal/lab/runner.go`, `internal/cluster/simulated_cluster.go`, `cmd/lab/main.go`, `cmd/lab/main_test.go`, `Makefile`

## References

- [system-design.md § State Machine](../system-design.md)
- [ADR: Scenarios are Go functions over immutable descriptions](2026-10-03-scenarios-as-go-functions.md)
- [ADR: The lab provisions the Simulated Cluster it drives](2026-10-03-lab-provisioned-simulated-cluster.md)
