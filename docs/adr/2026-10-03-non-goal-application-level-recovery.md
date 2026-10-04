# Non-goal: application-level recovery

**Status:** Accepted

## Context

"Resilience" covers two different questions. The first: does the control plane place and re-place a Workload's Pods the way its manifests declare? The second: does the application inside a Pod survive being restarted, drained, starved of memory, or cut off from its dependencies?

A Simulated Cluster answers only the first. Its control plane is genuine, but its Pods are fake and no container image is ever pulled, so there is no process to probe, shut down, or starve. A lab that claimed the second question would report green for behavior it never executed.

## Options Considered

1. **Placement only** — the lab asserts Placement and Required Capacity, and says so plainly.
2. **Placement plus a real-cluster profile** — a second run mode against a staging cluster, with a chaos tool injecting process-level faults.
3. **Placement plus simulated application state** — script Pod status transitions so a Pod can appear to fail its readiness check. The transitions are declared by the lab, so the assertion only re-reads its own input.

## Decision

The lab covers Placement under a declared Topology and Failure. Readiness and liveness behavior, graceful shutdown, networking, storage, resource pressure, and out-of-memory kills are out of scope. The project brief states where they belong: "Those belong to chaos tests on a real staging cluster (e.g. Chaos Mesh) and may be added later as a separate profile."

Option 3 is not rejected outright. Scripting Pod and Node status over time remains an open question in the PRD, for the sake of a more realistic Failure path — not as a claim about an application.

## Consequences

Positive: what a green run proves is unambiguous, and the lab's reports cannot be read as an application health statement. The scope stays small enough to run unattended in every build.

Negative: a team reading "resilience lab" may expect the application question answered, so the boundary has to be stated wherever the lab is introduced. A later staging-cluster profile would be a separate deliverable, not an extension of a Scenario.

## Implementation

**Non-goal:** NG-2

## References

- [NG-2: application-level recovery](../prd.md#non-goals) — the declined scope
- [ADR: Non-goal — real-cluster runs](2026-10-03-non-goal-real-cluster-runs.md) — the related boundary on where a Scenario runs
