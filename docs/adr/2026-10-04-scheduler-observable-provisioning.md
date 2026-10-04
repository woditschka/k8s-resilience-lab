# Provisioning ends when the scheduler has placed a Pod on every Node

**Status:** Accepted

## Context

A spread rule is evaluated once, when a Pod is scheduled, and never again.

The API server reports a Node Ready as soon as its status is committed. The scheduler places Pods from its own snapshot of those Nodes, and no API call shows that snapshot.

A Workload deployed while the snapshot is short of Nodes is spread evenly over the Nodes the scheduler had seen. Nine Pods then settle three per Zone at a Skew of two per Node, with no Pod Pending: a wrong answer rather than a late one. One run in three showed it, with the race detector on and the test packages running in parallel.

## Options Considered

1. **More agreeing observations of the API, or a wider poll interval** — the same rule, given longer.
2. **A minimum domain count on the Scenario's own constraints** — the scheduler told how many domains to expect.
3. **The settled Placement checked against the Workload's declared rules** — the wrong answer caught after the fact.
4. **A probe Pod per declared Node** — the scheduler itself asked, before the Workload is deployed.

## Decision

Option 4. `ApplyTopology` creates one probe Pod per declared Node, each selecting its Node by that Node's hostname label. It returns once every probe Pod has been Running and is removed. A Running probe Pod proves the scheduler holds its Node, because the scheduler binds only from its own snapshot. The wait polls at the poll interval within the start timeout, so no duration is guessed. The scheduler itself retries a probe Pod it cannot place until that Node arrives.

The wait stays inside `ApplyTopology`. A Workload deployed without it reports a wrong Placement, so no caller may skip it.

Option 1 is rejected because the snapshot is not API-observable: no agreement count over the API proves anything about it, and a wider interval is a guessed length.

Option 2 is rejected on the lab's own subject. While a domain is missing, a declared count makes the scheduler treat the global minimum as zero, so each domain holds one Pod at a maximum Skew of one: a lost Zone leaves seven of nine Pods Pending, and a lost Node leaves one. That is the outcome [ADR: A Scenario's Workload declares the spread of its own Pods](2026-10-04-scenario-declared-pod-spread.md) rejects Pod anti-affinity for.

Option 3 is rejected because a Node returning from a Failure legitimately leaves a Skew above the declared maximum, spread never being re-evaluated. The check would fail the Scenarios the lab exists to run.

## Consequences

Provisioning costs one Pod create, one observation loop and one delete per Node, within the start timeout.

A Node the scheduler never sees fails provisioning, named, instead of an even-looking Placement reported as success.

A Node holds at least one Pod by construction, so a probe Pod always fits. The probe is gone before a Workload is deployed, so a Topology too small for its Workload still reports Pending Pods.

A Scenario's own constraints are unchanged, so what the lab observes stays the scheduler's own answer.

## Implementation

**Requirements:** REQ-SIM-001

## References

- [system-design.md § Contracts](../system-design.md)
- [ADR: A Scenario's Workload declares the spread of its own Pods](2026-10-04-scenario-declared-pod-spread.md)
- [ADR: The lab is a Go program on a KWOK Simulated Cluster](2026-10-03-go-program-on-a-kwok-simulated-cluster.md)
