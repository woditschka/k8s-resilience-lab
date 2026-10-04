# Non-goal: real-cluster runs

**Status:** Accepted

## Context

The lab asserts how a Workload's Pods are placed and re-placed across Zones and Nodes. That assertion needs a Kubernetes control plane. Two sources of one exist: a Simulated Cluster the lab starts for the run, or a real cluster reached through a supplied address. An early project sketch allowed both, selecting the real cluster when an address was configured.

Allowing both doubles the behavior under test. A run against a real cluster depends on credentials, on the current contents of that cluster, and on the Nodes someone else provisioned, so the same Scenario can pass and fail for reasons the Scenario does not control.

## Options Considered

1. **Simulated Cluster only** — the lab provisions and destroys the cluster it uses. Every run has the same starting Topology.
2. **Simulated Cluster, with an opt-in real-cluster address** — one extra configuration path, exercised rarely, that changes what a passing Scenario proves.
3. **Real cluster only** — highest fidelity, no unattended run without a provisioned environment.

## Decision

The lab runs Scenarios against a Simulated Cluster only. The owner's decision: "it only should use the simulated kwok testcontainers instance". No Scenario reads a cluster address, and no run path reaches a cluster the lab did not create.

## Consequences

Positive: a run needs a container runtime and nothing else, so unattended runs behave identically on a developer machine and in a build. A Scenario's outcome depends only on the Topology and the Workload it declares. No cluster credential enters the project.

Negative: behavior that only a real cluster exhibits stays unverifiable here — see [ADR: Non-goal — application-level recovery](2026-10-03-non-goal-application-level-recovery.md). Fidelity now rests on how closely the declared Topology mirrors the environment a team cares about, which is an open question in the PRD.

## Implementation

**Non-goal:** NG-1

## References

- [NG-1: real-cluster runs](../prd.md#non-goals) — the declined scope
- [REQ-SIM-001: Simulated cluster lab](../prd.md#req-sim-001) — the requirement that depends on this boundary
